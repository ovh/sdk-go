// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// List and get: every DNS zone of the account with an iterator, then one zone and its records.
// Read-only.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//
//	go run ./examples/list-and-get
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/domain"
)

func main() {
	// WithReadOnly: this program cannot change anything, whatever its service account may do.
	client, err := ovh.NewClient(ovh.WithReadOnly())
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	zones := domain.New(client)

	// AllZones lists the names, then gets each zone. The next zone is read only when the loop
	// reaches it, so a break stops the calls.
	var names []string
	for zone, err := range zones.AllZones(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		last := "never"
		if zone.LastUpdate != nil {
			last = zone.LastUpdate.String()
		}
		fmt.Printf("%s last update %s\n", ovh.Value(zone.Name), last)
		if zone.Name != nil {
			names = append(names, *zone.Name)
		}
	}

	if len(names) == 0 {
		fmt.Println("no DNS zone")
		return
	}
	first := names[0]
	// One object, by its name.
	zone, err := zones.GetZone(ctx, first)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: name servers %s\n", ovh.Value(zone.Name), strings.Join(zone.NameServers, ", "))
	// The records of the zone, with a filter on the type.
	count := 0
	filter := domain.GetRecordsParams{FieldType: ovh.Ptr(domain.DomainZoneRecordTypeEnumA)}
	for record, err := range zones.AllRecords(ctx, first, filter) {
		if err != nil {
			log.Fatal(err)
		}
		count++
		sub := ovh.Value(record.SubDomain)
		if sub == "" {
			sub = "@"
		}
		fmt.Printf("  %s %s %s\n", sub, ovh.Value(record.FieldType), ovh.Value(record.Target))
	}
	fmt.Printf("%d A record(s)\n", count)
}
