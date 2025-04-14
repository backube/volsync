/*
Copyright 2020 The VolSync authors.

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

package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	volumepopulatorv1beta1 "github.com/kubernetes-csi/volume-data-source-validator/client/apis/volumepopulator/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover/rclone"
	"github.com/backube/volsync/internal/controller/mover/rsync"
	"github.com/backube/volsync/internal/controller/utils"
	//+kubebuilder:scaffold:imports
)

const (
	duration       = 10 * time.Second
	maxWait        = 60 * time.Second
	interval       = 250 * time.Millisecond
	dataVolumeName = "data"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg             *rest.Config
	k8sClient       client.Client
	k8sDirectClient client.Client
	testEnv         *envtest.Environment
	cancel          context.CancelFunc
	ctx             context.Context
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			// VolSync CRDs
			filepath.Join("..", "..", "config", "crd", "bases"),
			// Snapshot CRDs
			filepath.Join("..", "..", "hack", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = volsyncv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = snapv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = volumepopulatorv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	/*
		// From original boilerplate
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient).ToNot(BeNil())
	*/

	// Register the data movers
	Expect(rsync.Register()).To(Succeed())
	Expect(rclone.Register()).To(Succeed())
	//	Expect(restic.Register()).To(Succeed())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&ReplicationDestinationReconciler{
		Client:        k8sManager.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("Destination"),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: &record.FakeRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ReplicationSourceReconciler{
		Client:        k8sManager.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("Source"),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: &record.FakeRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// Index fields that are required for the VolumePopulator controller
	err = IndexFieldsForVolumePopulator(ctx, k8sManager.GetFieldIndexer())
	Expect(err).ToNot(HaveOccurred())

	err = (&VolumePopulatorReconciler{
		Client:        k8sManager.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("VolumePopulator"),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: &record.FakeRecorder{},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	// Instantiate direct client for tests (reads directly from API server rather than caching)
	k8sDirectClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	_, err = utils.InitPodLogsClient(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// beOwnedBy is a GomegaMatcher that ensures a Kubernetes Object is owned by a
// specific other object.
func beOwnedBy(owner interface{}) gomegatypes.GomegaMatcher {
	return &ownerRefMatcher{
		owner: owner,
	}
}

// Useful to avoid timing issues if the k8sclient must have the object
// created/updated etc in cache before the next step is run
// nolint:unparam
func createWithCacheReload(ctx context.Context, c client.Client, obj client.Object, intervals ...interface{}) {
	Expect(c.Create(ctx, obj)).To(Succeed())

	// Make sure the k8sClient cache has been updated with this change before returning
	Eventually(func() error {
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		return err
	}, getIntervals(intervals...)...).Should(Succeed())
}

func deleteWithCacheReload(ctx context.Context, c client.Client, obj client.Object, intervals ...interface{}) {
	err := c.Delete(ctx, obj)
	Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())

	// Make sure the k8sClient cache has been updated with this change before returning
	Eventually(func() bool {
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		return kerrors.IsNotFound(err)
	}, getIntervals(intervals...)...).Should(BeTrue())
}

func getIntervals(intervals ...interface{}) []interface{} {
	// Setting defaults that apply to our volsync tests
	if len(intervals) == 0 {
		return []interface{}{duration, interval}
	}

	if len(intervals) == 1 {
		return []interface{}{intervals[0], interval}
	}

	return []interface{}{intervals[0], intervals[1]}
}

type ownerRefMatcher struct {
	owner  interface{}
	reason string
}

func (m *ownerRefMatcher) Match(actual interface{}) (success bool, err error) {
	actObj, ok := actual.(metav1.Object)
	if !ok {
		return false, fmt.Errorf("actual value is not a metav1.Object")
	}
	ownerObj, ok := m.owner.(metav1.Object)
	if !ok {
		return false, fmt.Errorf("expected value is not a metav1.Object")
	}
	controller := metav1.GetControllerOf(actObj)
	if controller == nil {
		m.reason = "it does not have an owner"
		return false, nil
	}
	if controller.UID != ownerObj.GetUID() {
		m.reason = "it does not refer to the expected parent object"
		return false, nil
	}
	// XXX: This check isn't perfect. Both cluster-scoped and objects in the
	// "default" namespace have an empty namespace name. So the following may
	// (incorrectly) pass for namespaced owners in the default namespace
	// attempting to own cluster-scoped objects.
	if ownerObj.GetNamespace() != "" { // if owner not cluster-scoped
		if actObj.GetNamespace() != ownerObj.GetNamespace() {
			m.reason = "cross namespace owner references are not allowed"
			return false, nil
		}
	}
	return true, nil
}

func (m *ownerRefMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nto be owned by\n\t%#v\nbut %v", actual, m.owner, m.reason)
}

func (m *ownerRefMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nnot to be owned by\n\t%#v", actual, m.owner)
}
