/*
Copyright 2026 The VolSync authors.

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

package mover

// MoverTerminationGracePeriodSeconds is the terminationGracePeriodSeconds used
// for all Job-based mover Pods (Restic, rclone, rsync, rsync-TLS).
//
// Kubernetes defaults to 30s when the field is unset. Restic handles SIGTERM
// and bounds its lock-cleanup work with restic.UnlockCancelDelay (60s), so a
// grace period of 120s gives the forwarded stop signal time to release the
// repository lock plus a margin. The mover entrypoint scripts forward TERM to
// their child processes (see mover-common/shutdown.sh), which is what lets the
// cleanup run at all.
const MoverTerminationGracePeriodSeconds int64 = 120
