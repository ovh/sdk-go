// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// Create, then wait: a Public Cloud private network. The API only starts the work, so the
// program waits until the network is READY. Then it deletes the network and waits until it is
// gone. It changes the project: run it on a test project.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//	PROJECT_ID                         the Public Cloud project
//	CLOUD_REGION                       the region of the network, for example GRA11
//
//	go run ./examples/create-and-wait
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/publiccloud"
)

func main() {
	projectID, region := os.Getenv("PROJECT_ID"), os.Getenv("CLOUD_REGION")
	if projectID == "" || region == "" {
		fmt.Fprintln(os.Stderr, "set PROJECT_ID and CLOUD_REGION")
		os.Exit(2)
	}

	client, err := ovh.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pc := publiccloud.New(client)

	created, err := pc.CreateNetwork(ctx, projectID, publiccloud.PublicCloudNetworkNetworkCreation{
		TargetSpec: publiccloud.PublicCloudNetworkNetworkTargetSpec{
			Name:     "sdk-example",
			Location: publiccloud.PublicCloudCommonLocation{Region: region},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	networkID := ovh.Value(created.ID)
	if networkID == "" {
		log.Fatal("the API returned a network with no id")
	}
	fmt.Printf("network %s is %s\n", networkID, ovh.Value(created.ResourceStatus))

	get := func(ctx context.Context) (*publiccloud.PublicCloudNetworkNetwork, error) {
		return pc.GetNetwork(ctx, projectID, networkID)
	}
	// Polls every 2 seconds at first, then less often. Gives up after 10 minutes here.
	network, err := ovh.WaitReady(ctx, get, ovh.WaitTimeout(10*time.Minute))
	if errors.Is(err, ovh.ErrWaitFailed) {
		// The resource went to ERROR (or SUSPENDED...). network is its last state.
		fmt.Fprintf(os.Stderr, "the network failed: %v\n", err)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Printf("network %s is %s\n", networkID, ovh.Value(network.ResourceStatus))
	}

	// Delete it in all cases, then wait until the GET answers 404.
	if _, err := pc.DeleteNetwork(ctx, projectID, networkID); err != nil {
		log.Fatal(err)
	}
	if err := ovh.WaitGone(ctx, get); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("network %s deleted\n", networkID)
}
