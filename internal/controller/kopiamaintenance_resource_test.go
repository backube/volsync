/*
Copyright 2025 The VolSync authors.

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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

func TestResourceRequirementsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        corev1.ResourceRequirements
		b        corev1.ResourceRequirements
		expected bool
	}{
		{
			name:     "empty resources are equal",
			a:        corev1.ResourceRequirements{},
			b:        corev1.ResourceRequirements{},
			expected: true,
		},
		{
			name: "same memory limits are equal",
			a: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			b: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			expected: true,
		},
		{
			name: "different memory limits are not equal",
			a: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			b: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
			expected: false,
		},
		{
			name: "resources with and without limits are not equal",
			a: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			b:        corev1.ResourceRequirements{},
			expected: false,
		},
		{
			name: "equivalent values in different units are equal",
			a: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			b: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1024Mi"),
				},
			},
			expected: true,
		},
		{
			name: "same requests and limits are equal",
			a: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
			b: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
			expected: true,
		},
		{
			name: "different requests are not equal",
			a: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			b: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			expected: false,
		},
		{
			name: "missing resource type in one side is not equal",
			a: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
			},
			b: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resourceRequirementsEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("resourceRequirementsEqual() = %v, want %v", result, tt.expected)
				t.Logf("a: %+v", tt.a)
				t.Logf("b: %+v", tt.b)
			}
		})
	}
}

func TestPodSecurityContextEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *corev1.PodSecurityContext
		b        *corev1.PodSecurityContext
		expected bool
	}{
		{
			name:     "both nil are equal",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "nil and non-nil are not equal",
			a:        nil,
			b:        &corev1.PodSecurityContext{},
			expected: false,
		},
		{
			name:     "empty contexts are equal",
			a:        &corev1.PodSecurityContext{},
			b:        &corev1.PodSecurityContext{},
			expected: true,
		},
		{
			name: "same runAsUser are equal",
			a: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
			},
			b: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
			},
			expected: true,
		},
		{
			name: "different runAsUser are not equal",
			a: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
			},
			b: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(2000)),
			},
			expected: false,
		},
		{
			name: "same fsGroup and runAsUser are equal",
			a: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
				FSGroup:   ptr.To(int64(1000)),
			},
			b: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
				FSGroup:   ptr.To(int64(1000)),
			},
			expected: true,
		},
		{
			name: "different fsGroup are not equal",
			a: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
				FSGroup:   ptr.To(int64(1000)),
			},
			b: &corev1.PodSecurityContext{
				RunAsUser: ptr.To(int64(1000)),
				FSGroup:   ptr.To(int64(2000)),
			},
			expected: false,
		},
		{
			name: "same runAsNonRoot are equal",
			a: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
			},
			b: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
			},
			expected: true,
		},
		{
			name: "different runAsNonRoot are not equal",
			a: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
			},
			b: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(false),
			},
			expected: false,
		},
		{
			name: "complete default context matches",
			a: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				FSGroup:      ptr.To(int64(1000)),
				RunAsUser:    ptr.To(int64(1000)),
			},
			b: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				FSGroup:      ptr.To(int64(1000)),
				RunAsUser:    ptr.To(int64(1000)),
			},
			expected: true,
		},
		{
			name: "custom context with different user",
			a: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				FSGroup:      ptr.To(int64(2000)),
				RunAsUser:    ptr.To(int64(2000)),
			},
			b: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				FSGroup:      ptr.To(int64(1000)),
				RunAsUser:    ptr.To(int64(1000)),
			},
			expected: false,
		},
		{
			name: "same supplementalGroups are equal",
			a: &corev1.PodSecurityContext{
				SupplementalGroups: []int64{100, 200},
			},
			b: &corev1.PodSecurityContext{
				SupplementalGroups: []int64{100, 200},
			},
			expected: true,
		},
		{
			name: "different supplementalGroups are not equal",
			a: &corev1.PodSecurityContext{
				SupplementalGroups: []int64{100, 200},
			},
			b: &corev1.PodSecurityContext{
				SupplementalGroups: []int64{100, 300},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := podSecurityContextEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("podSecurityContextEqual() = %v, want %v", result, tt.expected)
				t.Logf("a: %+v", tt.a)
				t.Logf("b: %+v", tt.b)
			}
		})
	}
}