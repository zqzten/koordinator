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

package elasticquotatree

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	nrtinformers "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions"
	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	schedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	"k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schetesting "k8s.io/kubernetes/pkg/scheduler/testing"

	"github.com/koordinator-sh/koordinator/apis/extension"
	pgclientset "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/generated/clientset/versioned"
	pgfake "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/generated/clientset/versioned/fake"
	"github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
)

type ElasticQuotaSetAndHandle struct {
	framework.Handle
	pgclientset.Interface
}

func ElasticQuotaPluginFactoryProxy(clientSet pgclientset.Interface, factoryFn runtime.PluginFactory) runtime.PluginFactory {
	return func(args apiruntime.Object, handle framework.Handle) (framework.Plugin, error) {
		return factoryFn(args, ElasticQuotaSetAndHandle{Handle: handle, Interface: clientSet})
	}
}

func mockPodsList(w http.ResponseWriter, r *http.Request) {
	bear := r.Header.Get("Authorization")
	if bear == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	parts := strings.Split(bear, "Bearer")
	if len(parts) != 2 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	http_token := strings.TrimSpace(parts[1])
	if len(http_token) < 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if http_token != token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	podList := new(corev1.PodList)
	b, err := json.Marshal(podList)
	if err != nil {
		log.Printf("codec error %+v", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func parseHostAndPort(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "0", err
	}
	return net.SplitHostPort(u.Host)
}

var (
	token string
)

func newPluginTestSuit(t *testing.T) *pluginTestSuit {
	elasticQuotaPluginConfig := schedulerconfig.PluginConfig{
		Name: Name,
		Args: nil,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(mockPodsList))
	defer server.Close()

	address, portStr, err := parseHostAndPort(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &rest.Config{
		Host:        net.JoinHostPort(address, portStr),
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	koordClientSet := fake.NewSimpleClientset()
	koordSharedInformerFactory := koordinatorinformers.NewSharedInformerFactory(koordClientSet, 0)

	pgClientSet := pgfake.NewSimpleClientset()
	proxyNew := ElasticQuotaPluginFactoryProxy(pgClientSet, New)

	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)

	registeredPlugins := []schetesting.RegisterPluginFunc{
		func(reg *runtime.Registry, profile *schedulerconfig.KubeSchedulerProfile) {
			profile.PluginConfig = []schedulerconfig.PluginConfig{
				elasticQuotaPluginConfig,
			}
		},
		schetesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schetesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}

	handle, _ := schetesting.NewFramework(context.TODO(), registeredPlugins, "koord-scheduler",
		runtime.WithKubeConfig(cfg),
		runtime.WithInformerFactory(informerFactory),
		runtime.WithClientSet(cs))
	return &pluginTestSuit{
		Handle:                           handle,
		koordinatorSharedInformerFactory: koordSharedInformerFactory,
		proxyNew:                         proxyNew,
		client:                           pgClientSet,
	}
}

type pluginTestSuit struct {
	framework.Handle
	framework.Framework
	koordinatorSharedInformerFactory koordinatorinformers.SharedInformerFactory
	nrtSharedInformerFactory         nrtinformers.SharedInformerFactory
	proxyNew                         runtime.PluginFactory
	client                           *pgfake.Clientset
}

func TestACKQuotaTreeAdaptor_OnQuotaTreeAdd(t *testing.T) {
	suit := newPluginTestSuit(t)
	pl, _ := suit.proxyNew(nil, suit.Handle)
	p := pl.(*Plugin)
	p.OnQuotaTreeAdd(createQuotaTree1())
	time.Sleep(time.Second)
	eq, err := p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota1")
	assert.Nil(t, err)
	assert.NotNil(t, eq)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota2")
	assert.Nil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota3")
	assert.Nil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota4")
	assert.Nil(t, err)
}

func createQuotaTree1() *v1beta1.ElasticQuotaTree {
	tree := &v1beta1.ElasticQuotaTree{
		Spec: v1beta1.ElasticQuotaTreeSpec{
			Root: v1beta1.ElasticQuotaSpec{
				Name: extension.RootQuotaName,
				Children: []v1beta1.ElasticQuotaSpec{
					{
						Name:       "test1",
						Namespaces: []string{"quota1"},
					},
					{
						Name: "test2",
						Children: []v1beta1.ElasticQuotaSpec{
							{
								Name:       "child1",
								Namespaces: []string{"quota3"},
							},
						},
						Namespaces: []string{"quota2"},
					},
					{
						Name:       extension.RootQuotaName,
						Namespaces: []string{"quota4"},
					},
				},
			},
		},
	}
	return tree
}

func createQuotaTree2() *v1beta1.ElasticQuotaTree {
	tree := &v1beta1.ElasticQuotaTree{
		Spec: v1beta1.ElasticQuotaTreeSpec{
			Root: v1beta1.ElasticQuotaSpec{
				Name: extension.RootQuotaName,
				Children: []v1beta1.ElasticQuotaSpec{
					{
						Name:       "test3_tmp",
						Namespaces: []string{"quota5"},
					},
					{
						Name: "test4",
						Children: []v1beta1.ElasticQuotaSpec{
							{
								Name:       "child1",
								Namespaces: []string{"quota6"},
							},
						},
						Namespaces: []string{"quota7"},
					},
					{
						Name:       extension.RootQuotaName,
						Namespaces: []string{"quota4"},
					},
				},
			},
		},
	}
	return tree
}

func TestACKQuotaTreeAdaptor_OnQuotaTreeUpdate(t *testing.T) {
	suit := newPluginTestSuit(t)
	pl, _ := suit.proxyNew(nil, suit.Handle)
	p := pl.(*Plugin)

	p.OnQuotaTreeAdd(createQuotaTree1())
	time.Sleep(time.Second)

	p.OnQuotaTreeUpdate(nil, createQuotaTree2())
	time.Sleep(time.Second)

	_, err := p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota1")
	assert.NotNil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota2")
	assert.NotNil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota3")
	assert.NotNil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota5")
	assert.Nil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota6")
	assert.Nil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota7")
	assert.Nil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota4")
	assert.Nil(t, err)
}

func TestACKQuotaTreeAdaptor_OnQuotaTreeDelete(t *testing.T) {
	suit := newPluginTestSuit(t)
	pl, _ := suit.proxyNew(nil, suit.Handle)
	p := pl.(*Plugin)

	p.OnQuotaTreeAdd(createQuotaTree1())
	time.Sleep(time.Second)

	p.OnQuotaTreeDelete(createQuotaTree1())
	time.Sleep(time.Second)

	_, err := p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota1")
	assert.NotNil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota2")
	assert.NotNil(t, err)
	_, err = p.quotaLister.ElasticQuotas(ackElasticQuotaTreeNamespace).Get("quota3")
	assert.NotNil(t, err)
}
