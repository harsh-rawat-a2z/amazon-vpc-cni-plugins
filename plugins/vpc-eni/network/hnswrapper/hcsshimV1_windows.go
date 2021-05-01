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
	log "github.com/cihub/seelog"
)

// attachEndpointV1 attaches an endpoint to the container network compartment.
func (builder *builder) attachEndpointV1(ep *HNSEndpoint) error {
	log.Infof("Attaching HNS endpoint %s to container %s.", ep.ID, ep.Container.ContainerID)
	err := hcsshim.HotAttachEndpoint(ep.Container.ContainerID, ep.ID)
	if err != nil {
		// Attach can fail if the container is no longer running and/or its network namespace
		// has been cleaned up.
		log.Errorf("Failed to attach HNS endpoint %s: %v.", ep.ID, err)
	}

	return err
}

// deleteHNSEndpointV1 removes an endpoint from the container network compartment.
// For the infrastructure container, it deletes the endpoint as well.
func (builder *builder) deleteHNSEndpointV1(ep *HNSEndpoint) error {
	// Find the HNS Endpoint using its name.
	hnsEndpoint, err := hcsshim.GetHNSEndpointByName(ep.Name)
	if err != nil {
		return err
	}

	// Detach the HNS endpoint from the container's network namespace.
	log.Infof("Detaching HNS endpoint %s from container %s netns.", hnsEndpoint.Id, ep.Container.ContainerID)
	err = hcsshim.HotDetachEndpoint(ep.Container.ContainerID, hnsEndpoint.Id)
	// We should continue if infra container itself is not running.
	if err != nil && err != hcsshim.ErrComputeSystemDoesNotExist {
		return err
	}

	// The rest of the delete logic applies to infrastructure container only.
	if !ep.Container.IsInfraContainer {
		return nil
	}

	// Delete the HNS endpoint.
	log.Infof("Deleting HNS endpoint name: %s ID: %s.", ep.Name, hnsEndpoint.Id)
	_, err = hcsshim.HNSEndpointRequest("DELETE", hnsEndpoint.Id, "")
	if err != nil {
		log.Errorf("Failed to delete HNS endpoint: %v.", err)
	}

	return err
}
