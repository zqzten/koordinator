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

package fake

import (
	"context"

	v1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeResourcePolicies implements ResourcePolicyInterface
type FakeResourcePolicies struct {
	Fake *FakeSchedulingV1alpha1
}

var resourcepoliciesResource = v1alpha1.SchemeGroupVersion.WithResource("resourcepolicies")

var resourcepoliciesKind = v1alpha1.SchemeGroupVersion.WithKind("ResourcePolicy")

// Get takes name of the resourcePolicy, and returns the corresponding resourcePolicy object, and an error if there is any.
func (c *FakeResourcePolicies) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ResourcePolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(resourcepoliciesResource, name), &v1alpha1.ResourcePolicy{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourcePolicy), err
}

// List takes label and field selectors, and returns the list of ResourcePolicies that match those selectors.
func (c *FakeResourcePolicies) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ResourcePolicyList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(resourcepoliciesResource, resourcepoliciesKind, opts), &v1alpha1.ResourcePolicyList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ResourcePolicyList{ListMeta: obj.(*v1alpha1.ResourcePolicyList).ListMeta}
	for _, item := range obj.(*v1alpha1.ResourcePolicyList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested resourcePolicies.
func (c *FakeResourcePolicies) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(resourcepoliciesResource, opts))
}

// Create takes the representation of a resourcePolicy and creates it.  Returns the server's representation of the resourcePolicy, and an error, if there is any.
func (c *FakeResourcePolicies) Create(ctx context.Context, resourcePolicy *v1alpha1.ResourcePolicy, opts v1.CreateOptions) (result *v1alpha1.ResourcePolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(resourcepoliciesResource, resourcePolicy), &v1alpha1.ResourcePolicy{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourcePolicy), err
}

// Update takes the representation of a resourcePolicy and updates it. Returns the server's representation of the resourcePolicy, and an error, if there is any.
func (c *FakeResourcePolicies) Update(ctx context.Context, resourcePolicy *v1alpha1.ResourcePolicy, opts v1.UpdateOptions) (result *v1alpha1.ResourcePolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(resourcepoliciesResource, resourcePolicy), &v1alpha1.ResourcePolicy{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourcePolicy), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeResourcePolicies) UpdateStatus(ctx context.Context, resourcePolicy *v1alpha1.ResourcePolicy, opts v1.UpdateOptions) (*v1alpha1.ResourcePolicy, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(resourcepoliciesResource, "status", resourcePolicy), &v1alpha1.ResourcePolicy{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourcePolicy), err
}

// Delete takes name of the resourcePolicy and deletes it. Returns an error if one occurs.
func (c *FakeResourcePolicies) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(resourcepoliciesResource, name, opts), &v1alpha1.ResourcePolicy{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeResourcePolicies) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(resourcepoliciesResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.ResourcePolicyList{})
	return err
}

// Patch applies the patch and returns the patched resourcePolicy.
func (c *FakeResourcePolicies) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ResourcePolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(resourcepoliciesResource, name, pt, data, subresources...), &v1alpha1.ResourcePolicy{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourcePolicy), err
}
