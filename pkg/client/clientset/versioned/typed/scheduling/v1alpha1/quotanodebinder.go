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

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	"time"

	v1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	scheme "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// QuotaNodeBindersGetter has a method to return a QuotaNodeBinderInterface.
// A group's client should implement this interface.
type QuotaNodeBindersGetter interface {
	QuotaNodeBinders() QuotaNodeBinderInterface
}

// QuotaNodeBinderInterface has methods to work with QuotaNodeBinder resources.
type QuotaNodeBinderInterface interface {
	Create(ctx context.Context, quotaNodeBinder *v1alpha1.QuotaNodeBinder, opts v1.CreateOptions) (*v1alpha1.QuotaNodeBinder, error)
	Update(ctx context.Context, quotaNodeBinder *v1alpha1.QuotaNodeBinder, opts v1.UpdateOptions) (*v1alpha1.QuotaNodeBinder, error)
	UpdateStatus(ctx context.Context, quotaNodeBinder *v1alpha1.QuotaNodeBinder, opts v1.UpdateOptions) (*v1alpha1.QuotaNodeBinder, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.QuotaNodeBinder, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.QuotaNodeBinderList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.QuotaNodeBinder, err error)
	QuotaNodeBinderExpansion
}

// quotaNodeBinders implements QuotaNodeBinderInterface
type quotaNodeBinders struct {
	client rest.Interface
}

// newQuotaNodeBinders returns a QuotaNodeBinders
func newQuotaNodeBinders(c *SchedulingV1alpha1Client) *quotaNodeBinders {
	return &quotaNodeBinders{
		client: c.RESTClient(),
	}
}

// Get takes name of the quotaNodeBinder, and returns the corresponding quotaNodeBinder object, and an error if there is any.
func (c *quotaNodeBinders) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.QuotaNodeBinder, err error) {
	result = &v1alpha1.QuotaNodeBinder{}
	err = c.client.Get().
		Resource("quotanodebinders").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of QuotaNodeBinders that match those selectors.
func (c *quotaNodeBinders) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.QuotaNodeBinderList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.QuotaNodeBinderList{}
	err = c.client.Get().
		Resource("quotanodebinders").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested quotaNodeBinders.
func (c *quotaNodeBinders) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Resource("quotanodebinders").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a quotaNodeBinder and creates it.  Returns the server's representation of the quotaNodeBinder, and an error, if there is any.
func (c *quotaNodeBinders) Create(ctx context.Context, quotaNodeBinder *v1alpha1.QuotaNodeBinder, opts v1.CreateOptions) (result *v1alpha1.QuotaNodeBinder, err error) {
	result = &v1alpha1.QuotaNodeBinder{}
	err = c.client.Post().
		Resource("quotanodebinders").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(quotaNodeBinder).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a quotaNodeBinder and updates it. Returns the server's representation of the quotaNodeBinder, and an error, if there is any.
func (c *quotaNodeBinders) Update(ctx context.Context, quotaNodeBinder *v1alpha1.QuotaNodeBinder, opts v1.UpdateOptions) (result *v1alpha1.QuotaNodeBinder, err error) {
	result = &v1alpha1.QuotaNodeBinder{}
	err = c.client.Put().
		Resource("quotanodebinders").
		Name(quotaNodeBinder.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(quotaNodeBinder).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *quotaNodeBinders) UpdateStatus(ctx context.Context, quotaNodeBinder *v1alpha1.QuotaNodeBinder, opts v1.UpdateOptions) (result *v1alpha1.QuotaNodeBinder, err error) {
	result = &v1alpha1.QuotaNodeBinder{}
	err = c.client.Put().
		Resource("quotanodebinders").
		Name(quotaNodeBinder.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(quotaNodeBinder).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the quotaNodeBinder and deletes it. Returns an error if one occurs.
func (c *quotaNodeBinders) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Resource("quotanodebinders").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *quotaNodeBinders) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Resource("quotanodebinders").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched quotaNodeBinder.
func (c *quotaNodeBinders) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.QuotaNodeBinder, err error) {
	result = &v1alpha1.QuotaNodeBinder{}
	err = c.client.Patch(pt).
		Resource("quotanodebinders").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
