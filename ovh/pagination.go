// SPDX-FileCopyrightText: 2026 OVH SAS
//
// SPDX-License-Identifier: Apache-2.0

package ovh

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
)

// maxCursorPages bounds a v2 cursor walk, so a server handing out an endless supply of fresh
// cursors cannot loop forever.
const maxCursorPages = 200

// Iterate yields every item of a v2 cursor-paginated list, one at a time, fetching the next
// page only when the current one is used up. Generated AllX operations on such endpoints
// call it; it is their only form, so the caller decides whether to collect every item,
// filter them, or stop early.
//
// On an error it yields (nil, err) and stops. Breaking out of the range loop stops it too, so
// no further page is requested.
func Iterate[T any](ctx context.Context, c *Client, req *Request) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		page := *req
		page.Header = req.Header.Clone()
		if page.Header == nil {
			page.Header = http.Header{}
		}

		for range maxCursorPages {
			var items []T
			header, err := c.do(ctx, &page, &items)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range items {
				if !yield(&items[i], nil) {
					return
				}
			}

			next := header.Get("X-Pagination-Cursor-Next")
			if next == "" {
				return
			}
			page.Header.Set("X-Pagination-Cursor", next)
		}
		yield(nil, fmt.Errorf("ovh: %s %s: still paginating after %d pages", req.Method, req.Path, maxCursorPages))
	}
}

// Each yields the object behind every ID a v1 list returns, one at a time: it calls list once,
// then get for each ID as the caller asks for the next object. Generated AllX operations on v1
// lists call it, pairing a list (ListVracks) with its "get one" operation (GetVrack).
//
// An object that answers 404 was deleted between the list and its fetch; it is skipped. Any
// other error is yielded as (zero, err) and stops the iteration. Breaking out of the range
// loop stops it too, so no further object is requested.
func Each[ID, T any](ctx context.Context, list func(context.Context) ([]ID, error), get func(context.Context, ID) (T, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		ids, err := list(ctx)
		if err != nil {
			yield(zero, err)
			return
		}
		for _, id := range ids {
			item, err := get(ctx, id)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				yield(zero, err)
				return
			}
			if !yield(item, nil) {
				return
			}
		}
	}
}
