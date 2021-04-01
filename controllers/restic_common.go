/*
Copyright 2020 The Scribe authors.

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

package controllers

import (
	"fmt"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	"github.com/go-logr/logr"
)

func generateForgetOptions(inst scribev1alpha1.ReplicationSource, l logr.Logger) string {
	l.V(1).Info("generating FORGET_OPTIONS for restic data mover")

	if inst.Spec.Restic.Retain == nil {
		forgetOptions := fmt.Sprint("--keep-last ", 1)
		l.V(1).Info("when no retain is given: ", "FORGET_OPTIONS ", forgetOptions)
		return forgetOptions
	}
	foHourly := ""
	foDaily := ""
	foWeekly := ""
	foMonthly := ""
	foYearly := ""
	foWithin := ""
	if inst.Spec.Restic.Retain.Hourly != nil {
		foHourly = ("--keep-hourly ") + fmt.Sprint(*inst.Spec.Restic.Retain.Hourly)
	}
	if inst.Spec.Restic.Retain.Daily != nil {
		foDaily = fmt.Sprint("--keep-daily ", fmt.Sprint(*inst.Spec.Restic.Retain.Daily))

	}
	if inst.Spec.Restic.Retain.Weekly != nil {
		foWeekly = fmt.Sprint("--keep-weekly ", fmt.Sprint(*inst.Spec.Restic.Retain.Weekly))

	}
	if inst.Spec.Restic.Retain.Monthly != nil {
		foMonthly = fmt.Sprint("--keep-monthly ", fmt.Sprint(*inst.Spec.Restic.Retain.Monthly))

	}
	if inst.Spec.Restic.Retain.Yearly != nil {
		foYearly = fmt.Sprint("--keep-yearly ", fmt.Sprint(*inst.Spec.Restic.Retain.Yearly))

	}
	if inst.Spec.Restic.Retain.Within != nil {
		foWithin = fmt.Sprintf("%s%s", "--keep-within ", *inst.Spec.Restic.Retain.Within)

	}
	if foHourly == "" && foDaily == "" && foWeekly == "" && foYearly == "" && foWithin == "" {
		forgetOptions := fmt.Sprint("--keep-last ", 1)
		return forgetOptions
	}
	forgetOptions := foHourly + " " + foDaily + " " + foWeekly + " " + foMonthly + " " + foYearly + " " + foWithin

	l.V(1).Info("when retain is given: ", "FORGET_OPTIONS ", forgetOptions)
	return forgetOptions
}
