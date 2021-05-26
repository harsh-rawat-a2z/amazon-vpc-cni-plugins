// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package network

// netBuilder implements the Builder interface for Linux.
type netBuilder struct{}

// NewNetworkBuilder returns a new instance of NetBuilder.
func NewNetworkBuilder() Builder {
	return &netBuilder{}
}

// FindOrCreateNetwork creates a new network.
func (nb *netBuilder) FindOrCreateNetwork(nw *Network) error {
	return nil
}

// DeleteNetwork deletes an existing network.
func (nb *netBuilder) DeleteNetwork(nw *Network) error {
	return nil
}

// FindOrCreateEndpoint creates a new endpoint in the network.
func (nb *netBuilder) FindOrCreateEndpoint(nw *Network, ep *Endpoint) error {
	return nil
}

// DeleteEndpoint deletes an existing endpoint.
func (nb *netBuilder) DeleteEndpoint(nw *Network, ep *Endpoint) error {
	return nil
}
