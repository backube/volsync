//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia Cache Limits", func() {
	Describe("calculateCacheLimits", func() {
		Context("when cacheCapacity is nil", func() {
			It("should return zeros", func() {
				m := &Mover{cacheCapacity: nil}
				metaMB, contentMB := m.calculateCacheLimits()
				Expect(metaMB).To(Equal(int32(0)))
				Expect(contentMB).To(Equal(int32(0)))
			})
		})

		Context("when cacheCapacity is 1Gi", func() {
			It("should calculate 70% for metadata and 20% for content", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{cacheCapacity: capacity}
				metaMB, contentMB := m.calculateCacheLimits()
				Expect(metaMB).To(Equal(int32(716)))    // 1024 * 0.70
				Expect(contentMB).To(Equal(int32(204))) // 1024 * 0.20
			})
		})

		Context("when cacheCapacity is 5Gi", func() {
			It("should calculate 70% for metadata and 20% for content", func() {
				capacity := resource.NewQuantity(5*1024*1024*1024, resource.BinarySI)
				m := &Mover{cacheCapacity: capacity}
				metaMB, contentMB := m.calculateCacheLimits()
				Expect(metaMB).To(Equal(int32(3584)))    // 5120 * 0.70
				Expect(contentMB).To(Equal(int32(1024))) // 5120 * 0.20
			})
		})

		Context("when cacheCapacity is small (100Mi)", func() {
			It("should calculate correctly", func() {
				capacity := resource.NewQuantity(100*1024*1024, resource.BinarySI)
				m := &Mover{cacheCapacity: capacity}
				metaMB, contentMB := m.calculateCacheLimits()
				Expect(metaMB).To(Equal(int32(70)))    // 100 * 0.70
				Expect(contentMB).To(Equal(int32(20))) // 100 * 0.20
			})
		})
	})

	Describe("addCacheLimitEnvVars", func() {
		int32Ptr := func(i int32) *int32 { return &i }

		Context("when explicit limits are provided", func() {
			It("should use the explicit limits", func() {
				m := &Mover{
					metadataCacheSizeLimitMB: int32Ptr(2000),
					contentCacheSizeLimitMB:  int32Ptr(500),
					cacheCapacity:            nil,
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).To(HaveLen(2))
				Expect(envMap["KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"]).To(Equal("2000"))
				Expect(envMap["KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"]).To(Equal("500"))
			})
		})

		Context("when limits are nil but capacity is set", func() {
			It("should auto-calculate limits from capacity", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					metadataCacheSizeLimitMB: nil,
					contentCacheSizeLimitMB:  nil,
					cacheCapacity:            capacity,
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).To(HaveLen(2))
				Expect(envMap["KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"]).To(Equal("716"))
				Expect(envMap["KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"]).To(Equal("204"))
			})
		})

		Context("when limits are explicitly set to zero", func() {
			It("should not add env vars (unlimited)", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					metadataCacheSizeLimitMB: int32Ptr(0),
					contentCacheSizeLimitMB:  int32Ptr(0),
					cacheCapacity:            capacity,
				}
				envVars := m.addCacheLimitEnvVars(nil)
				Expect(envVars).To(BeEmpty())
			})
		})

		Context("when limits are nil and capacity is nil", func() {
			It("should not add env vars", func() {
				m := &Mover{
					metadataCacheSizeLimitMB: nil,
					contentCacheSizeLimitMB:  nil,
					cacheCapacity:            nil,
				}
				envVars := m.addCacheLimitEnvVars(nil)
				Expect(envVars).To(BeEmpty())
			})
		})

		Context("when mixing explicit and auto-calculated limits", func() {
			It("should use explicit for one and auto-calculate for the other", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					metadataCacheSizeLimitMB: int32Ptr(1000),
					contentCacheSizeLimitMB:  nil,
					cacheCapacity:            capacity,
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).To(HaveLen(2))
				Expect(envMap["KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"]).To(Equal("1000"))
				Expect(envMap["KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"]).To(Equal("204"))
			})
		})
	})
})

var _ = Describe("Kopia Cache Skip Logic", func() {
	int32Ptr := func(i int32) *int32 { return &i }

	Describe("shouldSkipCacheConfig", func() {
		Context("when no previous configuration exists", func() {
			It("should return false when both lastMetadata and lastContent are nil in source status", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: nil,
						LastConfiguredContentCacheSizeLimitMB:  nil,
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})

			It("should return false when both lastMetadata and lastContent are nil in destination status", func() {
				m := &Mover{
					isSource: false,
					destinationStatus: &volsyncv1alpha1.ReplicationDestinationKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: nil,
						LastConfiguredContentCacheSizeLimitMB:  nil,
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})

			It("should return false when source status is nil", func() {
				m := &Mover{
					isSource:     true,
					sourceStatus: nil,
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})

			It("should return false when destination status is nil", func() {
				m := &Mover{
					isSource:          false,
					destinationStatus: nil,
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})
		})

		Context("when both limits match previous values", func() {
			It("should return true for source", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeTrue())
			})

			It("should return true for destination", func() {
				m := &Mover{
					isSource: false,
					destinationStatus: &volsyncv1alpha1.ReplicationDestinationKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(2000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(800),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(2000), int32Ptr(800))
				Expect(result).To(BeTrue())
			})
		})

		Context("when metadata limit changed", func() {
			It("should return false for source", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(2000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})

			It("should return false for destination", func() {
				m := &Mover{
					isSource: false,
					destinationStatus: &volsyncv1alpha1.ReplicationDestinationKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(3000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})
		})

		Context("when content limit changed", func() {
			It("should return false for source", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(800))
				Expect(result).To(BeFalse())
			})

			It("should return false for destination", func() {
				m := &Mover{
					isSource: false,
					destinationStatus: &volsyncv1alpha1.ReplicationDestinationKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(1000))
				Expect(result).To(BeFalse())
			})
		})

		Context("when both are nil (no limits configured, no previous limits)", func() {
			It("should return false when no previous config and current is nil", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: nil,
						LastConfiguredContentCacheSizeLimitMB:  nil,
					},
				}
				result := m.shouldSkipCacheConfig(nil, nil)
				Expect(result).To(BeFalse())
			})
		})

		Context("when only metadata has previous config (content is nil)", func() {
			It("should return false when content limit changed from nil to value", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  nil,
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})

			It("should return true when metadata matches and both contents are nil", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  nil,
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), nil)
				Expect(result).To(BeTrue())
			})
		})

		Context("when handling nil vs non-nil transitions", func() {
			It("should return false when metadata changed from nil to value", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: nil,
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), int32Ptr(500))
				Expect(result).To(BeFalse())
			})

			It("should return false when content changed from value to nil", func() {
				m := &Mover{
					isSource: true,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(1000),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(500),
					},
				}
				result := m.shouldSkipCacheConfig(int32Ptr(1000), nil)
				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("addCacheLimitEnvVars with skip logic", func() {
		Context("when limits match previous configuration", func() {
			It("should set KOPIA_SKIP_CACHE_CONFIG=true for source", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 true,
					metadataCacheSizeLimitMB: int32Ptr(716),
					contentCacheSizeLimitMB:  int32Ptr(204),
					cacheCapacity:            capacity,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(716),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(204),
					},
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).To(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
				Expect(envMap["KOPIA_SKIP_CACHE_CONFIG"]).To(Equal("true"))
				// Verify limit vars are also present (they should always be set)
				Expect(envMap).To(HaveKey("KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"))
				Expect(envMap["KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"]).To(Equal("716"))
				Expect(envMap).To(HaveKey("KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"))
				Expect(envMap["KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"]).To(Equal("204"))
			})

			It("should set KOPIA_SKIP_CACHE_CONFIG=true for destination", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 false,
					metadataCacheSizeLimitMB: int32Ptr(716),
					contentCacheSizeLimitMB:  int32Ptr(204),
					cacheCapacity:            capacity,
					destinationStatus: &volsyncv1alpha1.ReplicationDestinationKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(716),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(204),
					},
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).To(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
				Expect(envMap["KOPIA_SKIP_CACHE_CONFIG"]).To(Equal("true"))
				// Verify limit vars are also present (they should always be set)
				Expect(envMap).To(HaveKey("KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"))
				Expect(envMap["KOPIA_METADATA_CACHE_SIZE_LIMIT_MB"]).To(Equal("716"))
				Expect(envMap).To(HaveKey("KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"))
				Expect(envMap["KOPIA_CONTENT_CACHE_SIZE_LIMIT_MB"]).To(Equal("204"))
			})
		})

		Context("when no previous configuration exists", func() {
			It("should NOT set KOPIA_SKIP_CACHE_CONFIG for source", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 true,
					metadataCacheSizeLimitMB: int32Ptr(716),
					contentCacheSizeLimitMB:  int32Ptr(204),
					cacheCapacity:            capacity,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: nil,
						LastConfiguredContentCacheSizeLimitMB:  nil,
					},
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).NotTo(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
			})

			It("should NOT set KOPIA_SKIP_CACHE_CONFIG when status is nil", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 true,
					metadataCacheSizeLimitMB: int32Ptr(716),
					contentCacheSizeLimitMB:  int32Ptr(204),
					cacheCapacity:            capacity,
					sourceStatus:             nil,
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).NotTo(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
			})

			It("should NOT set KOPIA_SKIP_CACHE_CONFIG when destination status is nil", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 false,
					metadataCacheSizeLimitMB: int32Ptr(716),
					contentCacheSizeLimitMB:  int32Ptr(204),
					cacheCapacity:            capacity,
					destinationStatus:        nil,
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).NotTo(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
			})
		})

		Context("when limits have changed", func() {
			It("should NOT set KOPIA_SKIP_CACHE_CONFIG when metadata limit changed", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 true,
					metadataCacheSizeLimitMB: int32Ptr(2000),
					contentCacheSizeLimitMB:  int32Ptr(204),
					cacheCapacity:            capacity,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(716),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(204),
					},
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).NotTo(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
			})

			It("should NOT set KOPIA_SKIP_CACHE_CONFIG when content limit changed", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 true,
					metadataCacheSizeLimitMB: int32Ptr(716),
					contentCacheSizeLimitMB:  int32Ptr(500),
					cacheCapacity:            capacity,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(716),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(204),
					},
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).NotTo(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
			})
		})

		Context("when using auto-calculated limits", func() {
			It("should skip when auto-calculated limits match previous", func() {
				capacity := resource.NewQuantity(1024*1024*1024, resource.BinarySI)
				m := &Mover{
					isSource:                 true,
					metadataCacheSizeLimitMB: nil,
					contentCacheSizeLimitMB:  nil,
					cacheCapacity:            capacity,
					sourceStatus: &volsyncv1alpha1.ReplicationSourceKopiaStatus{
						LastConfiguredMetadataCacheSizeLimitMB: int32Ptr(716),
						LastConfiguredContentCacheSizeLimitMB:  int32Ptr(204),
					},
				}
				envVars := m.addCacheLimitEnvVars(nil)

				envMap := make(map[string]string)
				for _, ev := range envVars {
					envMap[ev.Name] = ev.Value
				}

				Expect(envMap).To(HaveKey("KOPIA_SKIP_CACHE_CONFIG"))
				Expect(envMap["KOPIA_SKIP_CACHE_CONFIG"]).To(Equal("true"))
			})
		})
	})

	Describe("updateCacheLimitStatus", func() {
		Context("when updating source status", func() {
			It("should update source status correctly", func() {
				m := &Mover{
					isSource:                     true,
					sourceStatus:                 &volsyncv1alpha1.ReplicationSourceKopiaStatus{},
					calculatedMetadataCacheLimit: int32Ptr(1000),
					calculatedContentCacheLimit:  int32Ptr(500),
				}
				m.updateCacheLimitStatus()

				Expect(m.sourceStatus.LastConfiguredMetadataCacheSizeLimitMB).To(Equal(int32Ptr(1000)))
				Expect(m.sourceStatus.LastConfiguredContentCacheSizeLimitMB).To(Equal(int32Ptr(500)))
			})

			It("should handle nil calculated limits", func() {
				m := &Mover{
					isSource:                     true,
					sourceStatus:                 &volsyncv1alpha1.ReplicationSourceKopiaStatus{},
					calculatedMetadataCacheLimit: nil,
					calculatedContentCacheLimit:  nil,
				}
				m.updateCacheLimitStatus()

				Expect(m.sourceStatus.LastConfiguredMetadataCacheSizeLimitMB).To(BeNil())
				Expect(m.sourceStatus.LastConfiguredContentCacheSizeLimitMB).To(BeNil())
			})
		})

		Context("when updating destination status", func() {
			It("should update destination status correctly", func() {
				m := &Mover{
					isSource:                     false,
					destinationStatus:            &volsyncv1alpha1.ReplicationDestinationKopiaStatus{},
					calculatedMetadataCacheLimit: int32Ptr(2000),
					calculatedContentCacheLimit:  int32Ptr(800),
				}
				m.updateCacheLimitStatus()

				Expect(m.destinationStatus.LastConfiguredMetadataCacheSizeLimitMB).To(Equal(int32Ptr(2000)))
				Expect(m.destinationStatus.LastConfiguredContentCacheSizeLimitMB).To(Equal(int32Ptr(800)))
			})

			It("should handle nil calculated limits", func() {
				m := &Mover{
					isSource:                     false,
					destinationStatus:            &volsyncv1alpha1.ReplicationDestinationKopiaStatus{},
					calculatedMetadataCacheLimit: nil,
					calculatedContentCacheLimit:  nil,
				}
				m.updateCacheLimitStatus()

				Expect(m.destinationStatus.LastConfiguredMetadataCacheSizeLimitMB).To(BeNil())
				Expect(m.destinationStatus.LastConfiguredContentCacheSizeLimitMB).To(BeNil())
			})
		})

		Context("when status is nil", func() {
			It("should handle nil source status gracefully", func() {
				m := &Mover{
					isSource:                     true,
					sourceStatus:                 nil,
					calculatedMetadataCacheLimit: int32Ptr(1000),
					calculatedContentCacheLimit:  int32Ptr(500),
				}
				// Should not panic
				Expect(func() { m.updateCacheLimitStatus() }).NotTo(Panic())
			})

			It("should handle nil destination status gracefully", func() {
				m := &Mover{
					isSource:                     false,
					destinationStatus:            nil,
					calculatedMetadataCacheLimit: int32Ptr(1000),
					calculatedContentCacheLimit:  int32Ptr(500),
				}
				// Should not panic
				Expect(func() { m.updateCacheLimitStatus() }).NotTo(Panic())
			})
		})
	})
})
