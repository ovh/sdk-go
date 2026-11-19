// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// Account: the details of your account, GET /v1/me. The program of the README's quick start.
// Read-only.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//
//	go run ./examples/account
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/me"
)

func main() {
	client, err := ovh.NewClient() // reads OVH_CLIENT_ID, OVH_CLIENT_SECRET, OVH_REGION
	if err != nil {
		log.Fatal(err)
	}

	account, err := me.New(client).GetAccountDetails(context.Background()) // GET /v1/me
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("account:", ovh.Value(account.Nichandle))
	fmt.Println("email:  ", ovh.Value(account.Email))
	fmt.Println("country:", ovh.Value(account.Country))
}
