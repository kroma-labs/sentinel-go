package sql

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/kroma-labs/sentinel-go/sql/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapDriver(t *testing.T) {
	type args struct {
		opts []Option
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "given driver with options, then returns wrapped driver",
			args: args{opts: []Option{WithDBSystem("postgresql")}},
		},
		{
			name: "given driver without options, then returns wrapped driver",
			args: args{opts: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConn := mocks.NewDriverConn(t)
			mockDrv := &testDriver{conn: mockConn}

			wrapped := WrapDriver(mockDrv, tt.args.opts...)

			require.NotNil(t, wrapped)
			assert.Implements(t, (*driver.Driver)(nil), wrapped)
		})
	}
}

// testDriver is a simple driver that returns a mock connection.
type testDriver struct {
	conn    driver.Conn
	openErr error
}

func (d *testDriver) Open(_ string) (driver.Conn, error) {
	if d.openErr != nil {
		return nil, d.openErr
	}
	return d.conn, nil
}

func TestOtelDriver_Open(t *testing.T) {
	type args struct {
		dsn     string
		openErr error
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given successful open, then returns wrapped connection",
			args: args{
				dsn:     "test-dsn",
				openErr: nil,
			},
			wantErr: assert.NoError,
		},
		{
			name: "given error on open, then returns error",
			args: args{
				dsn:     "test-dsn",
				openErr: assert.AnError,
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockConn *mocks.DriverConn
			if tt.args.openErr == nil {
				mockConn = mocks.NewDriverConn(t)
			}
			mockDrv := &testDriver{conn: mockConn, openErr: tt.args.openErr}
			cfg := newConfig(WithDBSystem("postgresql"))
			otelDrv := &otelDriver{driver: mockDrv, cfg: cfg}

			conn, err := otelDrv.Open(tt.args.dsn)

			tt.wantErr(t, err)
			if err == nil {
				require.NotNil(t, conn)
				assert.IsType(t, &otelConn{}, conn)
			}
		})
	}
}

func TestOtelDriver_OpenConnector(t *testing.T) {
	t.Run("given driver without DriverContext, then returns dsnConnector", func(t *testing.T) {
		mockConn := mocks.NewDriverConn(t)
		mockDrv := &testDriver{conn: mockConn}
		cfg := newConfig(WithDBSystem("postgresql"))
		otelDrv := &otelDriver{driver: mockDrv, cfg: cfg}

		dsn := "test-dsn"
		connector, err := otelDrv.OpenConnector(dsn)

		require.NoError(t, err)
		require.NotNil(t, connector)
		assert.IsType(t, &dsnConnector{}, connector)
	})
}

func TestDsnConnector_Connect(t *testing.T) {
	type args struct {
		dsn     string
		openErr error
	}

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "given valid dsn, then returns wrapped connection",
			args: args{
				dsn:     "test-dsn",
				openErr: nil,
			},
			wantErr: assert.NoError,
		},
		{
			name: "given error on connect, then returns error",
			args: args{
				dsn:     "test-dsn",
				openErr: assert.AnError,
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockConn *mocks.DriverConn
			if tt.args.openErr == nil {
				mockConn = mocks.NewDriverConn(t)
			}
			mockDrv := &testDriver{conn: mockConn, openErr: tt.args.openErr}
			cfg := newConfig(WithDBSystem("postgresql"))
			otelDrv := &otelDriver{driver: mockDrv, cfg: cfg}
			connector := &dsnConnector{dsn: tt.args.dsn, driver: otelDrv}

			ctx := context.TODO()
			conn, err := connector.Connect(ctx)

			tt.wantErr(t, err)
			if err == nil {
				require.NotNil(t, conn)
				assert.IsType(t, &otelConn{}, conn)
			} else {
				assert.Nil(t, conn)
			}
		})
	}
}

func TestDsnConnector_Driver(t *testing.T) {
	t.Run("returns parent otelDriver", func(t *testing.T) {
		mockConn := mocks.NewDriverConn(t)
		mockDrv := &testDriver{conn: mockConn}
		cfg := newConfig()
		otelDrv := &otelDriver{driver: mockDrv, cfg: cfg}

		dsn := "test"
		connector := &dsnConnector{dsn: dsn, driver: otelDrv}

		got := connector.Driver()

		assert.Equal(t, otelDrv, got)
	})
}
