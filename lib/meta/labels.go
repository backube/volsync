package meta

import (
	"fmt"

	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation"
)

func ValidateLabels(labels map[string]string) (err error) {
	var errs []error
	for key, val := range labels {
		err := validation.IsQualifiedName(key)
		if len(err) > 0 {
			errs = append(errs, fmt.Errorf("label key %s is not a valid qualified name", key))
		}
		err = validation.IsValidLabelValue(val)
		if len(err) > 0 {
			errs = append(errs, fmt.Errorf("label value %s for key %s is not valid", key, val))
		}
	}
	return errorsutil.NewAggregate(errs)
}
