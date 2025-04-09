/*
Copyright 2022 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package utils

import (
	"bufio"
	"context"
	"io"
	"strings"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

const (
	// Env var - max bytes we will log into the status log
	MoverLogMaxBytesEnvVar      = "MOVER_LOG_MAX_BYTES"
	DefaultMoverLogMaxBytes int = 1024

	// Env var - max lines we will tail from a mover pod to process logs
	// Set to -1 to tail all lines since the mover pod start
	MoverLogTailLinesEnvVar        = "MOVER_LOG_TAIL_LINES"
	DefaultMoverLogTailLines int64 = -1

	// Env var - Set to "true" to log all lines (up to MOVER_LOG_MAX_LINES) of mover logs
	MoverLogDebugEnvVar = "MOVER_LOG_DEBUG"
)

//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get;list;watch

var clientset *kubernetes.Clientset

func InitPodLogsClient(cfg *rest.Config) (*kubernetes.Clientset, error) {
	var err error

	// Allow env var to override MOVER_LOG_MAX_BYTES
	// MOVER_LOG_MAX_BYTES is the maximum size in bytes of the filtered mover
	// log that will be saved to the status.latestMoverStatus.
	viper.SetDefault(MoverLogMaxBytesEnvVar, DefaultMoverLogMaxBytes)
	err = viper.BindEnv(MoverLogMaxBytesEnvVar)
	if err != nil {
		return nil, err
	}

	// Allow env var to override MOVER_LOG_TAIL_LINES
	// Note: this is actually the amt of lines we will tail from the
	// mover pod - so really it's the max lines we'll look at (and perhaps
	// filter) before saving to status.latestMoverStatus.
	// Logs will be filtered according to the mover and then be written to
	// the mover status.  Depending on the MOVER_LOG_MAX_BYTES setting, the
	// filtered log may still get truncated before saving to the status.
	viper.SetDefault(MoverLogTailLinesEnvVar, DefaultMoverLogTailLines)
	err = viper.BindEnv(MoverLogTailLinesEnvVar)
	if err != nil {
		return nil, err
	}

	// Allow env var to override MOVER_LOG_DEBUG
	// Set to "true" to log all lines (up to MOVER_LOG_MAX_BYTES) of mover logs
	// to status.latestMoverStatus.  This effectively bypasses mover filters.
	viper.SetDefault(MoverLogDebugEnvVar, "false")
	err = viper.BindEnv(MoverLogDebugEnvVar)
	if err != nil {
		return nil, err
	}

	clientset, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func GetMoverLogTailLines() int64 {
	return viper.GetInt64(MoverLogTailLinesEnvVar)
}

func GetMoverLogMaxBytes() int {
	return viper.GetInt(MoverLogMaxBytesEnvVar)
}

func IsMoverLogDebug() bool {
	return viper.GetBool(MoverLogDebugEnvVar)
}

func getPodLogs(ctx context.Context, logger logr.Logger, podName, podNamespace string,
	lineFilter func(line string) *string) (string, error) {
	l := logger.WithValues("podName", podName, "podNamespace", podNamespace)

	podLogOptions := &corev1.PodLogOptions{
		//Container: containerName,
		Follow: false,
	}

	tailLines := GetMoverLogTailLines()
	if tailLines >= 0 {
		podLogOptions.TailLines = &tailLines
	}

	request := clientset.CoreV1().Pods(podNamespace).GetLogs(podName, podLogOptions)
	stream, err := request.Stream(ctx)
	if err != nil {
		l.Error(err, "Error streaming logs from pod")
		return "", err
	}
	defer stream.Close()

	return FilterLogs(stream, lineFilter)
}

// Appies lineFilter to each line
func FilterLogs(reader io.Reader, lineFilter func(line string) *string) (string, error) {
	if IsMoverLogDebug() {
		// If in debug mode, log everything
		lineFilter = AllLines
	}

	lineScanner := bufio.NewScanner(reader)
	var allLines strings.Builder
	for lineScanner.Scan() {
		// Run lineFilter() func to see if the line should be appended
		lineAfterFilter := lineFilter(lineScanner.Text())

		if lineAfterFilter != nil {
			if allLines.Len() > 0 {
				allLines.WriteString("\n")
			}
			allLines.WriteString(*lineAfterFilter)
		}
	}
	if err := lineScanner.Err(); err != nil {
		return allLines.String(), err
	}
	return allLines.String(), nil
}

// Updates mover status to failed and puts the errMessage as the logs
func UpdateMoverStatusFailed(moverStatus *volsyncv1alpha1.MoverStatus, errMessage string) {
	moverStatus.Result = volsyncv1alpha1.MoverResultFailed
	moverStatus.Logs = errMessage
}

func UpdateMoverStatusForFailedJob(ctx context.Context, logger logr.Logger,
	moverStatus *volsyncv1alpha1.MoverStatus, jobName, jobNamespace string, logLineFilter func(string) *string) {
	updateMoverStatusForJob(ctx, logger, moverStatus, jobName, jobNamespace, true, logLineFilter)
}

func UpdateMoverStatusForSuccessfulJob(ctx context.Context, logger logr.Logger,
	moverStatus *volsyncv1alpha1.MoverStatus, jobName, jobNamespace string, logLineFilter func(string) *string) {
	updateMoverStatusForJob(ctx, logger, moverStatus, jobName, jobNamespace, false, logLineFilter)
}

// Does not throw error to avoid breaking movers from proceeding if logs can't be gathered
func updateMoverStatusForJob(ctx context.Context, logger logr.Logger, moverStatus *volsyncv1alpha1.MoverStatus,
	jobName, jobNamespace string, jobFailed bool, logLineFilter func(string) *string) {
	l := logger.WithValues("jobName", jobName)

	if logLineFilter == nil {
		// Default to printing all lines
		logLineFilter = AllLines
	}

	moverStatus.Logs = "" // clear out logs in case we can't get new ones

	moverStatus.Result = volsyncv1alpha1.MoverResultSuccessful
	if jobFailed {
		moverStatus.Result = volsyncv1alpha1.MoverResultFailed
	}

	pod, err := GetNewestPodForJob(ctx, logger, jobName, jobNamespace, jobFailed)
	if err != nil {
		l.Error(err, "Unable to get pod for job to get mover logs")
		return
	}

	if pod == nil {
		l.Info("No mover pods found to get logs from")
		return
	}

	l.Info("Getting logs for pod", "podName", pod.GetName(), "pod", pod)
	filteredLogs, err := getPodLogs(ctx, l, pod.GetName(), jobNamespace, logLineFilter)
	if err != nil {
		l.Error(err, "Error getting logs from pod")
	}

	moverStatus.Logs = truncateMoverLog(filteredLogs)
}

func truncateMoverLog(moverLog string) string {
	maxBytes := GetMoverLogMaxBytes()

	return TruncateString(moverLog, maxBytes)
}

func TruncateString(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) > maxBytes {
		s = string([]byte(s)[len(s)-maxBytes:])
	}
	return s
}

// Attempts to get the newest successful pod when jobFailed==false
// Attempts to get the newest failed pod (or newest running pod if no failed pods) if jobFailed==true
func GetNewestPodForJob(ctx context.Context, logger logr.Logger,
	jobName, jobNamespace string, jobFailed bool) (*corev1.Pod, error) {
	runningPods, successfulPods, failedPods, err := GetPodsForJob(ctx, logger, jobName, jobNamespace)
	if err != nil {
		return nil, err
	}

	var pod *corev1.Pod
	if jobFailed {
		pod = getNewestPod(failedPods)

		if pod == nil {
			// Try to get logs from latest running Pod instead
			pod = getNewestPod(runningPods)
		}
	} else {
		// Job was successful, try to get the logs from latest successful pod
		pod = getNewestPod(successfulPods)
	}
	return pod, nil
}

func GetPodsForJob(ctx context.Context, logger logr.Logger, jobName,
	jobNamespace string) (runningPods []corev1.Pod, successfulPods []corev1.Pod, failedPods []corev1.Pod, err error) {
	runningPods = []corev1.Pod{}
	successfulPods = []corev1.Pod{}
	failedPods = []corev1.Pod{}

	// Get pods for this job by label
	listOptions := metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	}

	podList, err := clientset.CoreV1().Pods(jobNamespace).List(ctx, listOptions)
	if err != nil {
		logger.Error(err, "Unable to list pods for job")
		return runningPods, successfulPods, failedPods, err
	}

	if len(podList.Items) == 0 {
		logger.Info("No pods found for job")
		return runningPods, successfulPods, failedPods, nil
	}

	for _, pod := range podList.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			runningPods = append(runningPods, pod)
		case corev1.PodSucceeded:
			successfulPods = append(successfulPods, pod)
		case corev1.PodFailed:
			failedPods = append(failedPods, pod)
		case corev1.PodPending:
			// no-op
		case corev1.PodUnknown:
			// no-op
		}
	}

	return runningPods, successfulPods, failedPods, nil
}

func getNewestPod(pods []corev1.Pod) *corev1.Pod {
	var newestPod *corev1.Pod
	for i := range pods {
		if newestPod == nil {
			newestPod = &pods[i]
		} else {
			if newestPod.CreationTimestamp.Before(&pods[i].CreationTimestamp) {
				newestPod = &pods[i]
			}
		}
	}

	return newestPod
}

func AllLines(line string) *string {
	return &line
}
