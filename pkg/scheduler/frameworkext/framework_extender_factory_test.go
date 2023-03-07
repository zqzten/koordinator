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

package frameworkext

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/services"
)

type fakeSharedLister struct {
	framework.SharedLister
}

func TestExtenderFactory(t *testing.T) {
	koordClientSet := koordfake.NewSimpleClientset()
	koordSharedInformerFactory := koordinformers.NewSharedInformerFactory(koordClientSet, 0)
	factory, err := NewFrameworkExtenderFactory(
		WithServicesEngine(services.NewEngine(gin.New())),
		WithKoordinatorClientSet(koordClientSet),
		WithKoordinatorSharedInformerFactory(koordSharedInformerFactory),
		WithSharedListerFactory(func(lister framework.SharedLister) framework.SharedLister {
			return &fakeSharedLister{SharedLister: lister}
		}),
		WithDefaultTransformers(&TestTransformer{index: 1}),
	)
	assert.NoError(t, err)
	assert.NotNil(t, factory)
	assert.Equal(t, koordClientSet, factory.KoordinatorClientSet())
	assert.Equal(t, koordSharedInformerFactory, factory.KoordinatorSharedInformerFactory())

	proxyNew := PluginFactoryProxy(factory, func(args runtime.Object, f framework.Handle) (framework.Plugin, error) {
		return &TestTransformer{index: 2}, nil
	})
	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}
	fh, err := schedulertesting.NewFramework(
		registeredPlugins,
		"koord-scheduler",
	)
	assert.NoError(t, err)
	pl, err := proxyNew(nil, fh)
	assert.NoError(t, err)
	assert.NotNil(t, pl)
	assert.Equal(t, "TestTransformer", pl.Name())

	extender := factory.GetExtender("koord-scheduler")
	assert.NotNil(t, extender)
	impl := extender.(*frameworkExtenderImpl)
	assert.Len(t, impl.preFilterTransformers, 2)
	assert.Len(t, impl.filterTransformers, 2)
	assert.Len(t, impl.scoreTransformers, 2)
	lister := extender.SnapshotSharedLister()
	_, ok := lister.(*fakeSharedLister)
	assert.True(t, ok)
}
