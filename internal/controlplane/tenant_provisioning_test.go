package controlplane

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

// 新租户开通补偿：先登记行（取 ID），同事务内建目录建库
// 迁移（provisioner），失败整体回滚登记行；成功后回写实际 db_locator。
type fakeTenantProvisioner struct {
	provisioned []uint
	locators    map[uint]string
	err         error
}

func (p *fakeTenantProvisioner) ProvisionTenant(_ context.Context, tenant *Tenant) error {
	if p.err != nil {
		return p.err
	}
	p.provisioned = append(p.provisioned, tenant.ID)
	p.locators[tenant.ID] = "file:///data/tenants/" + strconv.Itoa(int(tenant.ID)) + "/db.sqlite"
	tenant.DMLocator = p.locators[tenant.ID]
	return nil
}

func TestCreateTenantProvisionsDatabaseInTransaction(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	service := NewService(repository)
	provisioner := &fakeTenantProvisioner{locators: map[uint]string{}}
	service.SetTenantProvisioner(provisioner)

	tenant, err := service.CreateTenant(context.Background(), &Actor{ID: 1, Username: "root"}, TenantInput{Slug: "acme", DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tenant.ID == 0 || len(provisioner.provisioned) != 1 || provisioner.provisioned[0] != tenant.ID {
		t.Fatalf("provisioner must observe the persisted tenant id: id=%d provisioned=%v", tenant.ID, provisioner.provisioned)
	}
	stored, err := repository.GetTenant(context.Background(), tenant.ID)
	if err != nil {
		t.Fatalf("reload tenant: %v", err)
	}
	if stored.DMLocator != provisioner.locators[tenant.ID] {
		t.Fatalf("db_locator must reflect the provisioned path: %q", stored.DMLocator)
	}
}

func TestCreateTenantRollsBackRowWhenProvisioningFails(t *testing.T) {
	repository := openTestControlPlane(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	service := NewService(repository)
	provisioner := &fakeTenantProvisioner{locators: map[uint]string{}, err: errors.New("disk full")}
	service.SetTenantProvisioner(provisioner)

	if _, err := service.CreateTenant(context.Background(), nil, TenantInput{Slug: "acme", DisplayName: "Acme"}); err == nil {
		t.Fatalf("provisioning failure must fail tenant creation")
	}
	existing, err := repository.GetTenantBySlug(context.Background(), "acme")
	if err != nil {
		t.Fatalf("query tenant by slug: %v", err)
	}
	if existing != nil {
		t.Fatalf("tenant row must roll back when provisioning fails, got %#v", existing)
	}
	// 回滚后 slug 释放，可重试成功。
	provisioner.err = nil
	if _, err := service.CreateTenant(context.Background(), nil, TenantInput{Slug: "acme", DisplayName: "Acme"}); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}
