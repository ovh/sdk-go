// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

// Ordering with a cart: a cart, a domain name in it, and the price of the order. It stops
// before payment: GetCartCheckout gives the price, CreateCartCheckout would place the order.
// Then it deletes the cart.
//
//	OVH_CLIENT_ID, OVH_CLIENT_SECRET   OAuth2 service-account credentials
//	OVH_REGION                         eu (default), ca or us
//	DOMAIN                             a domain name to price, for example my-new-name.com
//	SUBSIDIARY                         the OVHcloud subsidiary of the account: FR (default), GB...
//
//	go run ./examples/cart-order
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/ovh/sdk-go/ovh"
	"github.com/ovh/sdk-go/pkg/order"
)

func main() {
	name := os.Getenv("DOMAIN")
	if name == "" {
		fmt.Fprintln(os.Stderr, "set DOMAIN")
		os.Exit(2)
	}
	// Not checked here: the API refuses an unknown subsidiary.
	subsidiary := os.Getenv("SUBSIDIARY")
	if subsidiary == "" {
		subsidiary = "FR"
	}

	client, err := ovh.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	orders := order.New(client)

	cart, err := orders.CreateCart(ctx, order.OrderCartCreation{
		OvhSubsidiary: order.NichandleOvhSubsidiaryEnum(subsidiary),
		Description:   ovh.Ptr("sdk example"),
	})
	if err != nil {
		log.Fatal(err)
	}
	cartID := ovh.Value(cart.CartID)
	if cartID == "" {
		log.Fatal("the API returned a cart with no id")
	}
	defer func() {
		if err := orders.DeleteCart(ctx, cartID); err != nil {
			log.Print(err)
		}
	}()

	if err := price(ctx, orders, cartID, name); err != nil {
		log.Print(err)
	}
}

func price(ctx context.Context, orders *order.Service, cartID, name string) error {
	// What the domain offers: is it free, for how long, at which price.
	offers, err := orders.ListCartDomains(ctx, cartID, name)
	if err != nil {
		return err
	}
	var offer *order.OrderCartProductInformation
	for i := range offers {
		if ovh.Value(offers[i].Orderable) {
			offer = &offers[i]
			break
		}
	}
	if offer == nil {
		fmt.Printf("%s cannot be ordered\n", name)
		return nil
	}
	item := order.OrderCartGenericDomainCreation{Domain: name}
	if len(offer.Duration) > 0 {
		item.Duration = &offer.Duration[0]
	}
	if _, err := orders.CreateCartDomain(ctx, cartID, item); err != nil {
		return err
	}
	// The cart belongs to the account: needed before a checkout.
	if err := orders.AssignCart(ctx, cartID); err != nil {
		return err
	}
	checkout, err := orders.GetCartCheckout(ctx, cartID)
	if err != nil {
		return err
	}
	withTax := ""
	if checkout.Prices != nil && checkout.Prices.WithTax != nil {
		withTax = ovh.Value(checkout.Prices.WithTax.Text)
	}
	fmt.Printf("%s: %s with tax\n", name, withTax)
	return nil
}
