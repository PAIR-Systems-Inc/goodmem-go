package goodmem

import (
	"context"
	"iter"
)

// Page is a single page of a paginated list response plus the means to fetch
// the following pages on demand. The list methods (spaces.List, memories.List)
// return the first page; iterate every item across all pages with All:
//
//	page, err := client.Spaces().List(ctx, nil)
//	if err != nil { ... }
//	for sp, err := range page.All(ctx) {
//	    if err != nil { ... }
//	    fmt.Println(sp.SpaceID)
//	}
//
// Or walk page by page with Items()/Next().
type Page[T any] struct {
	items     []T
	nextToken string
	fetch     func(ctx context.Context, token string) ([]T, string, error)
}

// newPage eagerly fetches the first page so callers get an immediate error on a
// bad request rather than a deferred one mid-iteration.
func newPage[T any](ctx context.Context, fetch func(context.Context, string) ([]T, string, error)) (*Page[T], error) {
	items, token, err := fetch(ctx, "")
	if err != nil {
		return nil, err
	}
	return &Page[T]{items: items, nextToken: token, fetch: fetch}, nil
}

// Items returns the items on this page only.
func (p *Page[T]) Items() []T { return p.items }

// NextToken returns the pagination token for the following page ("" if none).
func (p *Page[T]) NextToken() string { return p.nextToken }

// HasMore reports whether another page is available.
func (p *Page[T]) HasMore() bool { return p.nextToken != "" }

// Next fetches the following page, or returns (nil, nil) when there are no more.
func (p *Page[T]) Next(ctx context.Context) (*Page[T], error) {
	if p.nextToken == "" {
		return nil, nil
	}
	items, token, err := p.fetch(ctx, p.nextToken)
	if err != nil {
		return nil, err
	}
	return &Page[T]{items: items, nextToken: token, fetch: p.fetch}, nil
}

// All returns a range-over-func iterator (Go 1.23+) over every item across all
// pages, fetching subsequent pages lazily. If a page fetch fails, the iterator
// yields the zero value with the error once and stops.
func (p *Page[T]) All(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		cur := p
		for {
			for _, item := range cur.items {
				if !yield(item, nil) {
					return
				}
			}
			if cur.nextToken == "" {
				return
			}
			next, err := cur.Next(ctx)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			cur = next
		}
	}
}
