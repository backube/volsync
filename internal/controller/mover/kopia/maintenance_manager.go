//go:build !disable_kopia

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

package kopia

// NOTE: EnhancedMaintenanceManager has been removed as part of simplifying
// the KopiaMaintenance CRD. The CRD is now namespace-scoped and no longer
// supports repository selectors or priority-based matching.
//
// Maintenance is now handled directly by:
// - KopiaMaintenanceReconciler for KopiaMaintenance CRDs
// - MaintenanceManager for legacy embedded maintenance configuration in ReplicationSources
