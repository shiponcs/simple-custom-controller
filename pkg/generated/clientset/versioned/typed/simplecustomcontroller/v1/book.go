/*
Copyright The Kubernetes Authors.

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
	context "context"

	simplecustomcontrollerv1 "github.com/shiponcs/simple-custom-controller/pkg/apis/simplecustomcontroller/v1"
	scheme "github.com/shiponcs/simple-custom-controller/pkg/generated/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// BooksGetter has a method to return a BookInterface.
// A group's client should implement this interface.
type BooksGetter interface {
	Books(namespace string) BookInterface
}

// BookInterface has methods to work with Book resources.
type BookInterface interface {
	Create(ctx context.Context, book *simplecustomcontrollerv1.Book, opts metav1.CreateOptions) (*simplecustomcontrollerv1.Book, error)
	Update(ctx context.Context, book *simplecustomcontrollerv1.Book, opts metav1.UpdateOptions) (*simplecustomcontrollerv1.Book, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, book *simplecustomcontrollerv1.Book, opts metav1.UpdateOptions) (*simplecustomcontrollerv1.Book, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*simplecustomcontrollerv1.Book, error)
	List(ctx context.Context, opts metav1.ListOptions) (*simplecustomcontrollerv1.BookList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *simplecustomcontrollerv1.Book, err error)
	BookExpansion
}

// books implements BookInterface
type books struct {
	*gentype.ClientWithList[*simplecustomcontrollerv1.Book, *simplecustomcontrollerv1.BookList]
}

// newBooks returns a Books
func newBooks(c *SimplecustomcontrollerV1Client, namespace string) *books {
	return &books{
		gentype.NewClientWithList[*simplecustomcontrollerv1.Book, *simplecustomcontrollerv1.BookList](
			"books",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *simplecustomcontrollerv1.Book { return &simplecustomcontrollerv1.Book{} },
			func() *simplecustomcontrollerv1.BookList { return &simplecustomcontrollerv1.BookList{} },
		),
	}
}
