package sql

import (
	"testing"

	"github.com/kroma-labs/sentinel-go/sql/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOtelTx(t *testing.T) {
	t.Run("given tx and config, then creates wrapped transaction", func(t *testing.T) {
		mockTx := mocks.NewDriverTx(t)
		cfg := newConfig(WithDBSystem("postgresql"))

		otelTx := newOtelTx(mockTx, cfg)

		require.NotNil(t, otelTx)
		assert.Equal(t, mockTx, otelTx.tx)
		assert.Equal(t, cfg, otelTx.cfg)
	})
}

func TestOtelTx_Commit(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(*mocks.DriverTx)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful commit, then returns nil",
			mockFn: func(m *mocks.DriverTx) {
				m.EXPECT().Commit().Return(nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given commit error, then returns error",
			mockFn: func(m *mocks.DriverTx) {
				m.EXPECT().Commit().Return(assert.AnError)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTx := mocks.NewDriverTx(t)
			tt.mockFn(mockTx)

			cfg := newConfig(WithDBSystem("postgresql"))
			otelTx := newOtelTx(mockTx, cfg)

			err := otelTx.Commit()

			tt.wantErr(t, err)
		})
	}
}

func TestOtelTx_Rollback(t *testing.T) {
	tests := []struct {
		name    string
		mockFn  func(*mocks.DriverTx)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful rollback, then returns nil",
			mockFn: func(m *mocks.DriverTx) {
				m.EXPECT().Rollback().Return(nil)
			},
			wantErr: assert.NoError,
		},
		{
			name: "given rollback error, then returns error",
			mockFn: func(m *mocks.DriverTx) {
				m.EXPECT().Rollback().Return(assert.AnError)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTx := mocks.NewDriverTx(t)
			tt.mockFn(mockTx)

			cfg := newConfig(WithDBSystem("postgresql"))
			otelTx := newOtelTx(mockTx, cfg)

			err := otelTx.Rollback()

			tt.wantErr(t, err)
		})
	}
}
