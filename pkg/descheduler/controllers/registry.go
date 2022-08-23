/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/unified/orchestratingslo"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/framework/runtime"
)

func NewControllerRegistry() runtime.Registry {
	return runtime.Registry{
		migration.Name:        migration.New,
		orchestratingslo.Name: orchestratingslo.New,
		drain.Name:            drain.New,
	}
}
