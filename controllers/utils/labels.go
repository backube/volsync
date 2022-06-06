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

const (
	volsyncLabelPrefix  = "volsync.backube"
	cleanupLabelKey     = volsyncLabelPrefix + "/cleanup"
	DoNotDeleteLabelKey = volsyncLabelPrefix + "/do-not-delete"
	OwnedByLabelKey     = volsyncLabelPrefix + "/owned"
)

type Labelable interface {
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
}

func HasLabel(obj Labelable, key string) bool {
	labels := obj.GetLabels()
	for k := range labels {
		if k == key {
			return true
		}
	}
	return false
}

func HasLabelWithValue(obj Labelable, key string, value string) bool {
	labels := obj.GetLabels()
	for k, v := range labels {
		if k == key && v == value {
			return true
		}
	}
	return false
}

// Ensures that a given key/value label is present and returns True if an update
// was made
func AddLabel(obj Labelable, key string, value string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if oldVal, ok := labels[key]; ok && oldVal == value {
		return false
	}
	labels[key] = value
	obj.SetLabels(labels)
	return true
}

func RemoveLabel(obj Labelable, key string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	if _, ok := labels[key]; !ok {
		return false
	}
	delete(labels, key)
	obj.SetLabels(labels)
	return true
}
