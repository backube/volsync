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
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RelationshipLabelKey is a label applied to objects that are created as a
	// part of a given relationship. The value of the key is the UUID of the
	// relationship.
	RelationshipLabelKey = "volsync.backube/relationship"
)

// Each relationship type (e.g., replication, migration, backup, etc.) should
// define its own type string so that the load/save routines can ensure that the
// config files are used with the correct relationship type.
type RelationshipType string

// Relationship is the low-level structure that represents a volsync
// relationship. Each specific type will define its own fields and wrap this
// struct.
type Relationship struct {
	viper.Viper
	name string
}

// createRelationship creates a new relationship structure. If an existing
// relationship file is found, this will return an error.
func createRelationship(configDir string, name string, rType RelationshipType) (*Relationship, error) {
	filename := path.Join(configDir, name) + ".yaml"
	if err := os.MkdirAll(configDir, 0711); err != nil {
		return nil, fmt.Errorf("unable to create configuration directory (%s): %w", configDir, err)
	}
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

// CreateRelationshipFromCommand wraps the relationship creation, automatically
// extracting the config dir and name from the command flags.
func CreateRelationshipFromCommand(cmd *cobra.Command, rType RelationshipType) (*Relationship, error) {
	configDir, err := cmd.Flags().GetString("config-dir")
	if err != nil {
		return nil, err
	}
	rName, err := cmd.Flags().GetString("relationship")
	if err != nil {
		return nil, err
	}
	return createRelationship(configDir, rName, rType)
}

// loadRelationship creates a relationship structure based on an existing
// relationship file. If the relationship does not exist or is of the wrong
// type, this function will return an error.
func loadRelationship(configDir string, name string, rType RelationshipType) (*Relationship, error) {
	filename := path.Join(configDir, name) + ".yaml"
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("relationship not found")
	}
	v := viper.New()
	v.SetConfigFile(filename)
	klog.V(1).Infof("loading relationship from %s", filename)
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

// LoadRelationshipFromCommand wraps the relationship loading, automatically
// extracting the config dir and name from the command flags.
func LoadRelationshipFromCommand(cmd *cobra.Command, rType RelationshipType) (*Relationship, error) {
	configDir, err := cmd.Flags().GetString("config-dir")
	if err != nil {
		return nil, err
	}
	rName, err := cmd.Flags().GetString("relationship")
	if err != nil {
		return nil, err
	}
	rel, err := loadRelationship(configDir, rName, rType)
	if err != nil {
		return nil, err
	}
	return rel, nil
}

// Save persists the relationship information into the associated relationship
// file. Prior to calling the save() method, the underlying Viper instance needs
// to be updated with the state that will be saved.
func (r *Relationship) Save() error {
	klog.V(1).Infof("saving relationship information to %s", r.ConfigFileUsed())
	return r.WriteConfig()
}

// Delete deletes a relationship's associated file.
func (r *Relationship) Delete() error {
	filename := r.ConfigFileUsed()
	klog.V(1).Infof("deleting relationship file %s", filename)
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

// ID returns the UUID of this relationship.
func (r *Relationship) ID() uuid.UUID {
	return uuid.MustParse(r.GetString("id"))
}

// Sets the "data" subkey with the contents of a struct and flattens it so that
// individual values may be overridden
func (r *Relationship) SetData(data interface{}) error {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	ms := map[string]interface{}{}
	if err = yaml.Unmarshal(bytes, &ms); err != nil {
		return err
	}

	// While we'd like to just be able to r.Set("data", ms) to update the
	// relationship data, Viper doesn't support deleting keys. This means that
	// if our data struct has a (newly) nil value in it, the old value will get
	// preserved. To work around this, we create a new Viper and swap it out
	// with the old.
	newViper := viper.New()
	newViper.SetConfigFile(r.ConfigFileUsed())
	for _, key := range r.AllKeys() {
		// Copy across all k/v except for the data keys that we are trying to
		// replace.
		if !strings.HasPrefix(key, "data") {
			newViper.Set(key, r.Get(key))
		}
	}
	newViper.Set("data", ms)
	r.Viper = *newViper
	return nil
}

// Unmarshals the "data" subkey back into the provided struct, properly decoding
// any embedded resource.Quantity fields.
func (r *Relationship) GetData(data interface{}) error {
	return r.UnmarshalKey("data", data, viper.DecodeHook(func(from reflect.Type,
		to reflect.Type,
		data interface{},
	) (interface{}, error) {
		if from.Kind() != reflect.String {
			return data, nil
		}
		if to != reflect.TypeOf(resource.Quantity{}) {
			return data, nil
		}
		// We know this is going to succeed due to from.Kind() check
		sQuantity, _ := data.(string)
		return resource.ParseQuantity(sQuantity)
	}))
}

func (r *Relationship) AddIDLabel(object client.Object) {
	labels := object.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[RelationshipLabelKey] = r.ID().String()
	object.SetLabels(labels)
}
