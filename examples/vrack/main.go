// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// vRack: every vRack of the account, with the Public Cloud projects in it. Read-only.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//
//	go run ./examples/vrack
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/vrack"
)

func main() {
	client, err := ovh.NewClient(ovh.WithReadOnly())
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	vracks := vrack.New(client)

	// ListVracks gives the names only. AllVracks gets each vRack too.
	count := 0
	for v, err := range vracks.AllVracks(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		count++
		name := ovh.Value(v.ServiceName)
		fmt.Printf("%s %q %s\n", name, ovh.Value(v.Name), ovh.Value(v.Description))
		projects, err := vracks.ListCloudProjects(ctx, name)
		if err != nil {
			log.Fatal(err)
		}
		for _, p := range projects {
			fmt.Printf("  Public Cloud project %s\n", p)
		}
	}
	fmt.Printf("%d vRack(s)\n", count)
}
