// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// Errors: an HTTP error is an *ovh.APIError. errors.Is matches the common statuses, errors.As
// gives every detail. Test them from the most precise to the most general. Read-only.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//
//	go run ./examples/errors
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/domain"
)

func main() {
	client, err := ovh.NewClient(ovh.WithReadOnly())
	if err != nil {
		log.Fatal(err)
	}

	// .invalid is a reserved name: this zone does not exist.
	zone, err := domain.New(client).GetZone(context.Background(), "no-such-zone.invalid")
	var apiErr *ovh.APIError
	switch {
	case err == nil:
		fmt.Printf("found %s\n", ovh.Value(zone.Name))
	case errors.Is(err, ovh.ErrNotFound) && errors.As(err, &apiErr):
		// 404: the object does not exist, or the account cannot see it.
		fmt.Printf("not found: %s %s: %s\n", apiErr.Method, apiErr.Path, apiErr.Message)
	case errors.Is(err, ovh.ErrForbidden) && errors.As(err, &apiErr):
		// 403: the IAM actions that the service account needs.
		fmt.Printf("forbidden: the service account needs %s\n", strings.Join(apiErr.IAMActions, ", "))
	case errors.As(err, &apiErr):
		// Any other HTTP error. Give the query id to the support.
		fmt.Printf("%s from %s, query id %s\n", apiErr.Status, apiErr.Operation, apiErr.QueryID)
	default:
		// Not an HTTP error: credentials, network, timeout, read-only mode...
		fmt.Println(err)
	}
}
