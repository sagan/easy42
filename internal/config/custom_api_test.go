package config

import (
	"testing"
)

func TestValidateCustomAPI(t *testing.T) {
	tests := []struct {
		name    string
		api     *CustomAPI
		wantErr bool
	}{
		{
			name:    "nil api",
			api:     nil,
			wantErr: true,
		},
		{
			name: "empty id",
			api: &CustomAPI{
				ID:       "",
				Template: "hello",
			},
			wantErr: true,
		},
		{
			name: "invalid id with spaces",
			api: &CustomAPI{
				ID:       "invalid id",
				Template: "hello",
			},
			wantErr: true,
		},
		{
			name: "invalid id with special characters",
			api: &CustomAPI{
				ID:       "id/with/slash",
				Template: "hello",
			},
			wantErr: true,
		},
		{
			name: "empty template",
			api: &CustomAPI{
				ID:       "valid-id",
				Template: "",
			},
			wantErr: true,
		},
		{
			name: "valid api with default content type",
			api: &CustomAPI{
				ID:       "my_api-123",
				Template: "{{ .nodes | toJson }}",
			},
			wantErr: false,
		},
		{
			name: "valid disabled api",
			api: &CustomAPI{
				ID:       "disabled-api",
				Template: "ok",
				Disabled: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCustomAPI(tt.api)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCustomAPI() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.api != nil && tt.api.ContentType == "" {
				t.Errorf("expected default ContentType, got empty")
			}
		})
	}
}

func TestFindCustomAPI(t *testing.T) {
	cfg := &Config{
		CustomAPIs: []CustomAPI{
			{ID: "api-1", ContentType: "application/json", Template: "test1"},
			{ID: "api-2", ContentType: "text/plain", Template: "test2"},
		},
	}

	found := cfg.FindCustomAPI("api-1")
	if found == nil || found.Template != "test1" {
		t.Fatalf("expected to find api-1, got %v", found)
	}

	found2 := cfg.FindCustomAPI("api-2")
	if found2 == nil || found2.Template != "test2" {
		t.Fatalf("expected to find api-2, got %v", found2)
	}

	missing := cfg.FindCustomAPI("api-missing")
	if missing != nil {
		t.Fatalf("expected nil for missing api, got %v", missing)
	}
}
