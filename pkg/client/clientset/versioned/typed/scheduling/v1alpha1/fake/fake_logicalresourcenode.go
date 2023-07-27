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
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeLogicalResourceNodes implements LogicalResourceNodeInterface
type FakeLogicalResourceNodes struct {
	Fake *FakeSchedulingV1alpha1
}

var logicalresourcenodesResource = schema.GroupVersionResource{Group: "scheduling.koordinator.sh", Version: "v1alpha1", Resource: "logicalresourcenodes"}

var logicalresourcenodesKind = schema.GroupVersionKind{Group: "scheduling.koordinator.sh", Version: "v1alpha1", Kind: "LogicalResourceNode"}

// Get takes name of the logicalResourceNode, and returns the corresponding logicalResourceNode object, and an error if there is any.
func (c *FakeLogicalResourceNodes) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.LogicalResourceNode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(logicalresourcenodesResource, name), &v1alpha1.LogicalResourceNode{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LogicalResourceNode), err
}

// List takes label and field selectors, and returns the list of LogicalResourceNodes that match those selectors.
func (c *FakeLogicalResourceNodes) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.LogicalResourceNodeList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(logicalresourcenodesResource, logicalresourcenodesKind, opts), &v1alpha1.LogicalResourceNodeList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.LogicalResourceNodeList{ListMeta: obj.(*v1alpha1.LogicalResourceNodeList).ListMeta}
	for _, item := range obj.(*v1alpha1.LogicalResourceNodeList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested logicalResourceNodes.
func (c *FakeLogicalResourceNodes) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(logicalresourcenodesResource, opts))
}

// Create takes the representation of a logicalResourceNode and creates it.  Returns the server's representation of the logicalResourceNode, and an error, if there is any.
func (c *FakeLogicalResourceNodes) Create(ctx context.Context, logicalResourceNode *v1alpha1.LogicalResourceNode, opts v1.CreateOptions) (result *v1alpha1.LogicalResourceNode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(logicalresourcenodesResource, logicalResourceNode), &v1alpha1.LogicalResourceNode{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LogicalResourceNode), err
}

// Update takes the representation of a logicalResourceNode and updates it. Returns the server's representation of the logicalResourceNode, and an error, if there is any.
func (c *FakeLogicalResourceNodes) Update(ctx context.Context, logicalResourceNode *v1alpha1.LogicalResourceNode, opts v1.UpdateOptions) (result *v1alpha1.LogicalResourceNode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(logicalresourcenodesResource, logicalResourceNode), &v1alpha1.LogicalResourceNode{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LogicalResourceNode), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeLogicalResourceNodes) UpdateStatus(ctx context.Context, logicalResourceNode *v1alpha1.LogicalResourceNode, opts v1.UpdateOptions) (*v1alpha1.LogicalResourceNode, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(logicalresourcenodesResource, "status", logicalResourceNode), &v1alpha1.LogicalResourceNode{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LogicalResourceNode), err
}

// Delete takes name of the logicalResourceNode and deletes it. Returns an error if one occurs.
func (c *FakeLogicalResourceNodes) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(logicalresourcenodesResource, name, opts), &v1alpha1.LogicalResourceNode{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeLogicalResourceNodes) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(logicalresourcenodesResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.LogicalResourceNodeList{})
	return err
}

// Patch applies the patch and returns the patched logicalResourceNode.
func (c *FakeLogicalResourceNodes) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.LogicalResourceNode, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(logicalresourcenodesResource, name, pt, data, subresources...), &v1alpha1.LogicalResourceNode{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LogicalResourceNode), err
}
