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

package hnswrapper

import (
	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/hcn"
	log "github.com/cihub/seelog"
)

// attachEndpointV2 attaches an endpoint to the network namespace.
func (builder *builder) attachEndpointV2(ep *HNSEndpoint) error {
	log.Infof("Adding HNS endpoint %s to ns %s.", ep.ID, ep.NetNS.NetNSName)

	// Check if endpoint is already in target namespace.
	nsEndpoints, err := hcn.GetNamespaceEndpointIds(ep.NetNS.NetNSName)
	if err != nil {
		log.Errorf("Failed to get endpoints from namespace %s: %v.", ep.NetNS.NetNSName, err)
		return err
	}
	for _, endpointID := range nsEndpoints {
		if endpointID == endpointID {
			log.Infof("HNS endpoint %s is already in ns %s.", endpointID, ep.NetNS.NetNSName)
			return nil
		}
	}

	// Add the endpoint to the target namespace.
	return hcn.AddNamespaceEndpoint(ep.NetNS.NetNSName, ep.ID)
}

// deleteHNSEndpointV2 deletes an endpoint from the network namespace.
func (builder *builder) deleteHNSEndpointV2(ep *HNSEndpoint) error {
	// Find the HNS Endpoint using its name.
	hnsEndpoint, err := hcsshim.GetHNSEndpointByName(ep.Name)
	if err != nil {
		return err
	}

	// Remove the HNS endpoint from the namespace.
	log.Infof("Removing HNS endpoint %s from ns %s.", hnsEndpoint.Id, ep.NetNS.NetNSName)
	err = hcn.RemoveNamespaceEndpoint(ep.NetNS.NetNSName, hnsEndpoint.Id)
	if err != nil {
		return err
	}

	// Delete the HNS endpoint.
	log.Infof("Deleting HNS endpoint name: %s ID: %s.", ep.Name, hnsEndpoint.Id)
	_, err = hnsEndpoint.Delete()
	if err != nil {
		log.Errorf("Failed to delete HNS endpoint: %v.", err)
	}

	return err
}
