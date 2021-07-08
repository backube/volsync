/*
Copyright 2021 The Scribe authors.

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

// Scribe data movers are created by implementing the interfaces in this
// package. Each data mover must implement the Builder interface that constructs
// instances of the Mover (interface) given a ReplicationSource or
// ReplicationDestination CR. These builders must be created and Register()-ed
// with the Catalog at startup time for that mover type to be available.
//
// When an RS or RD CR is reconciled, the Builders in the Catalog are tried in
// sequence. If one successfully returns a Mover, that mover is used to perform
// the reconcile.
//
// Movers implement the actual synchronization of data and return a Result from
// each invocation. When one of the Mover's functions returns Completed(), the
// operation (either synchronization or cleanup of a previous synchronization is
// considered to be completed).
package mover
