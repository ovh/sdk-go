// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// A DNS record: create a TXT record, then refresh the zone so the name servers serve it. A
// record is not served until the zone is refreshed. It changes the zone: use a test zone.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//	ZONE                               a DNS zone of the account, for example example.com
//
//	go run ./examples/dns-record
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/domain"
)

func main() {
	zone := os.Getenv("ZONE")
	if zone == "" {
		fmt.Fprintln(os.Stderr, "set ZONE")
		os.Exit(2)
	}

	client, err := ovh.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	dns := domain.New(client)

	record, err := dns.CreateRecord(ctx, zone, domain.DomainZoneRecordCreateWritable{
		FieldType: domain.DomainZoneRecordTypeEnumTxt,
		SubDomain: ovh.Ptr("sdk-example"),
		Target:    `"created by the OVHcloud Go SDK"`,
		TTL:       ovh.Ptr(60),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("record %d: %s.%s %s %s\n", ovh.Value(record.ID), ovh.Value(record.SubDomain), zone,
		ovh.Value(record.FieldType), ovh.Value(record.Target))

	// Apply the change: the name servers now serve the new record.
	if err := dns.RefreshZone(ctx, zone); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("zone %s refreshed\n", zone)

	// To remove it: DeleteRecord, then RefreshZone again.
	if os.Getenv("CLEANUP") != "" && record.ID != nil {
		if err := dns.DeleteRecord(ctx, zone, *record.ID); err != nil {
			log.Fatal(err)
		}
		if err := dns.RefreshZone(ctx, zone); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("record %d deleted\n", *record.ID)
	}
}
