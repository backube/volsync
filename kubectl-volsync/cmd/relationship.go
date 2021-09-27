/*
Copyright © 2021 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

type RelationshipType string

type Relationship struct {
	viper.Viper
	name string
}

// CreateRelationship creates a new relationship structure. If an existing
// relationship file is found, this will return an error.
func CreateRelationship(configDir string, name string, rType RelationshipType) (*Relationship, error) {
	filename := path.Join(configDir, name) + ".yaml"
	if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("unable to create relationship: relationship exists")
	}
	v := viper.New()
	v.SetConfigFile(filename)
	v.Set("type", string(rType))
	v.Set("id", uuid.New())
	return &Relationship{
		Viper: *v,
		name:  name,
	}, nil
}

// LoadRelationship creates a relationship structure based on an existing
// relationship file. If the relationship does not exist or is of the wrong
// type, this function will return an error.
func LoadRelationship(configDir string, name string, rType RelationshipType) (*Relationship, error) {
	filename := path.Join(configDir, name) + ".yaml"
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("relationship not found")
	}
	v := viper.New()
	v.SetConfigFile(filename)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("loading relationship: %w", err)
	}
	if rType != RelationshipType(v.GetString("type")) {
		return nil, fmt.Errorf("relationship is not of the correct type")
	}
	return &Relationship{
		Viper: *v,
		name:  name,
	}, nil
}

// Save persists the relationship information into the associated relationship
// file.
func (r *Relationship) Save() error {
	return r.WriteConfig()
}

// Delete deletes a relationship's associated file.
func (r *Relationship) Delete() error {
	filename := r.Viper.ConfigFileUsed()
	return os.Remove(filename)
}

// Name retrieves the name of this relationship.
func (r *Relationship) Name() string {
	return r.name
}

// Type returns the type of this relationship.
func (r *Relationship) Type() RelationshipType {
	return RelationshipType(r.GetString("type"))
}
