package server

import (
	"testing"

	"buf.build/go/protovalidate"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
)

func TestPageRequestValidation(t *testing.T) {
	v, err := protovalidate.New()
	if err != nil {
		t.Fatalf("protovalidate.New: %v", err)
	}

	tests := []struct {
		name    string
		req     *zerxv1.PageRequest
		wantErr bool
	}{
		{name: "zero defaults", req: &zerxv1.PageRequest{}, wantErr: false},
		{name: "page_size upper bound", req: &zerxv1.PageRequest{Page: 1, PageSize: 100}, wantErr: false},
		{name: "page_size over limit", req: &zerxv1.PageRequest{PageSize: 1000}, wantErr: true},
		{name: "negative page_size", req: &zerxv1.PageRequest{PageSize: -1}, wantErr: true},
		{name: "negative page", req: &zerxv1.PageRequest{Page: -1}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%v) err = %v, wantErr %v", tt.req, err, tt.wantErr)
			}
		})
	}
}
