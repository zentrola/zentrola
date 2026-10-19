package postgres

import (
	"testing"

	"github.com/zentrola/zentrola/internal/domain/operation"
)

func TestOperationLogAcceptsResourceQuotaRefresh(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	if _, err := pool.Exec(ctx, `INSERT INTO operation_log (id,operator_type,operator_id,operator_name,module,operation_type,target_type,target_id,result,created_at)
VALUES (300,'ADMIN',70,'Admin','RESOURCE',$1,'RESOURCE',61,'SUCCESS',now())`, string(operation.ResourceQuotaRefresh)); err != nil {
		t.Fatal(err)
	}
}
