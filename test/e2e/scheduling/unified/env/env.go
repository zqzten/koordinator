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

package env

import (
	"os"
)

const (
	TesterJituan = "jituan"
)

var Tester string

func init() {
	Tester = GetTester()
}

// GetTester return the tester, it could be jituan or ant.
func GetTester() string {
	get, ok := os.LookupEnv("TESTER")
	if ok {
		return get
	}
	return TesterJituan
}
