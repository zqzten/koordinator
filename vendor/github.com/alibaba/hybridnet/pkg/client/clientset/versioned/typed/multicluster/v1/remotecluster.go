/*
Copyright 2021 The Hybridnet Authors.

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
// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	v1 "github.com/alibaba/hybridnet/pkg/apis/multicluster/v1"
	scheme "github.com/alibaba/hybridnet/pkg/client/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// RemoteClustersGetter has a method to return a RemoteClusterInterface.
// A group's client should implement this interface.
type RemoteClustersGetter interface {
	RemoteClusters() RemoteClusterInterface
}

// RemoteClusterInterface has methods to work with RemoteCluster resources.
type RemoteClusterInterface interface {
	Create(ctx context.Context, remoteCluster *v1.RemoteCluster, opts metav1.CreateOptions) (*v1.RemoteCluster, error)
	Update(ctx context.Context, remoteCluster *v1.RemoteCluster, opts metav1.UpdateOptions) (*v1.RemoteCluster, error)
	UpdateStatus(ctx context.Context, remoteCluster *v1.RemoteCluster, opts metav1.UpdateOptions) (*v1.RemoteCluster, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.RemoteCluster, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.RemoteClusterList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.RemoteCluster, err error)
	RemoteClusterExpansion
}

// remoteClusters implements RemoteClusterInterface
type remoteClusters struct {
	client rest.Interface
}

// newRemoteClusters returns a RemoteClusters
func newRemoteClusters(c *MulticlusterV1Client) *remoteClusters {
	return &remoteClusters{
		client: c.RESTClient(),
	}
}

// Get takes name of the remoteCluster, and returns the corresponding remoteCluster object, and an error if there is any.
func (c *remoteClusters) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.RemoteCluster, err error) {
	result = &v1.RemoteCluster{}
	err = c.client.Get().
		Resource("remoteclusters").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of RemoteClusters that match those selectors.
func (c *remoteClusters) List(ctx context.Context, opts metav1.ListOptions) (result *v1.RemoteClusterList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1.RemoteClusterList{}
	err = c.client.Get().
		Resource("remoteclusters").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested remoteClusters.
func (c *remoteClusters) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("remoteclusters").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a remoteCluster and creates it.  Returns the server's representation of the remoteCluster, and an error, if there is any.
func (c *remoteClusters) Create(ctx context.Context, remoteCluster *v1.RemoteCluster, opts metav1.CreateOptions) (result *v1.RemoteCluster, err error) {
	result = &v1.RemoteCluster{}
	err = c.client.Post().
		Resource("remoteclusters").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(remoteCluster).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a remoteCluster and updates it. Returns the server's representation of the remoteCluster, and an error, if there is any.
func (c *remoteClusters) Update(ctx context.Context, remoteCluster *v1.RemoteCluster, opts metav1.UpdateOptions) (result *v1.RemoteCluster, err error) {
	result = &v1.RemoteCluster{}
	err = c.client.Put().
		Resource("remoteclusters").
		Name(remoteCluster.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(remoteCluster).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *remoteClusters) UpdateStatus(ctx context.Context, remoteCluster *v1.RemoteCluster, opts metav1.UpdateOptions) (result *v1.RemoteCluster, err error) {
	result = &v1.RemoteCluster{}
	err = c.client.Put().
		Resource("remoteclusters").
		Name(remoteCluster.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(remoteCluster).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the remoteCluster and deletes it. Returns an error if one occurs.
func (c *remoteClusters) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return c.client.Delete().
		Resource("remoteclusters").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *remoteClusters) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("remoteclusters").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched remoteCluster.
func (c *remoteClusters) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.RemoteCluster, err error) {
	result = &v1.RemoteCluster{}
	err = c.client.Patch(pt).
		Resource("remoteclusters").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
