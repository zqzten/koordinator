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

// FakeResourceFlavors implements ResourceFlavorInterface
type FakeResourceFlavors struct {
	Fake *FakeSchedulingV1alpha1
}

var resourceflavorsResource = v1alpha1.SchemeGroupVersion.WithResource("resourceflavors")

var resourceflavorsKind = v1alpha1.SchemeGroupVersion.WithKind("ResourceFlavor")

// Get takes name of the resourceFlavor, and returns the corresponding resourceFlavor object, and an error if there is any.
func (c *FakeResourceFlavors) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ResourceFlavor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(resourceflavorsResource, name), &v1alpha1.ResourceFlavor{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourceFlavor), err
}

// List takes label and field selectors, and returns the list of ResourceFlavors that match those selectors.
func (c *FakeResourceFlavors) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ResourceFlavorList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(resourceflavorsResource, resourceflavorsKind, opts), &v1alpha1.ResourceFlavorList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ResourceFlavorList{ListMeta: obj.(*v1alpha1.ResourceFlavorList).ListMeta}
	for _, item := range obj.(*v1alpha1.ResourceFlavorList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested resourceFlavors.
func (c *FakeResourceFlavors) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(resourceflavorsResource, opts))
}

// Create takes the representation of a resourceFlavor and creates it.  Returns the server's representation of the resourceFlavor, and an error, if there is any.
func (c *FakeResourceFlavors) Create(ctx context.Context, resourceFlavor *v1alpha1.ResourceFlavor, opts v1.CreateOptions) (result *v1alpha1.ResourceFlavor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(resourceflavorsResource, resourceFlavor), &v1alpha1.ResourceFlavor{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourceFlavor), err
}

// Update takes the representation of a resourceFlavor and updates it. Returns the server's representation of the resourceFlavor, and an error, if there is any.
func (c *FakeResourceFlavors) Update(ctx context.Context, resourceFlavor *v1alpha1.ResourceFlavor, opts v1.UpdateOptions) (result *v1alpha1.ResourceFlavor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(resourceflavorsResource, resourceFlavor), &v1alpha1.ResourceFlavor{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourceFlavor), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeResourceFlavors) UpdateStatus(ctx context.Context, resourceFlavor *v1alpha1.ResourceFlavor, opts v1.UpdateOptions) (*v1alpha1.ResourceFlavor, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(resourceflavorsResource, "status", resourceFlavor), &v1alpha1.ResourceFlavor{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourceFlavor), err
}

// Delete takes name of the resourceFlavor and deletes it. Returns an error if one occurs.
func (c *FakeResourceFlavors) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(resourceflavorsResource, name, opts), &v1alpha1.ResourceFlavor{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeResourceFlavors) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(resourceflavorsResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.ResourceFlavorList{})
	return err
}

// Patch applies the patch and returns the patched resourceFlavor.
func (c *FakeResourceFlavors) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ResourceFlavor, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(resourceflavorsResource, name, pt, data, subresources...), &v1alpha1.ResourceFlavor{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ResourceFlavor), err
}
