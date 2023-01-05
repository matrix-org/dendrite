package shared

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/syncapi/types"
)

func TestDatabaseTransaction_GetRoomSummary(t *testing.T) {
	type fields struct {
		Database *Database
		ctx      context.Context
		txn      *sql.Tx
	}
	type args struct {
		ctx    context.Context
		roomID string
		userID string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *types.Summary
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DatabaseTransaction{
				Database: tt.fields.Database,
				ctx:      tt.fields.ctx,
				txn:      tt.fields.txn,
			}
			got, err := d.GetRoomSummary(tt.args.ctx, tt.args.roomID, tt.args.userID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetRoomSummary() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetRoomSummary() got = %v, want %v", got, tt.want)
			}
		})
	}
}
