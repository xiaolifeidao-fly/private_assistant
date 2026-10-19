package viewer

import (
	"context"
	"testing"
)

func withSystem(t *testing.T, accounts ...string) {
	previous := SystemAccounts
	SystemAccounts = func(context.Context) []string { return accounts }
	t.Cleanup(func() { SystemAccounts = previous })
}

func TestClause(t *testing.T) {
	withSystem(t, "acc_me")
	grandma := With(context.Background(), Viewer{Account: "acc_nai", Locked: true})

	clause, args := Clause(grandma, "m.")
	if clause != "(m.visibility = 'family' OR m.owner_account IN ?)" || len(args) != 1 || args[0].([]string)[0] != "acc_nai" {
		t.Fatalf("登录账号只看全家可见的和自己的，得到 %s %v", clause, args)
	}
	if _, args := Clause(AsSystem(context.Background()), ""); args[0].([]string)[0] != "acc_me" {
		t.Fatalf("系统身份应按我自己的账号看，得到 %v", args)
	}
	if clause, _ := Clause(Unscoped(grandma), ""); clause != "1 = 1" {
		t.Fatalf("Unscoped 不应过滤，得到 %s", clause)
	}
	if _, args := Clause(AsSystem(grandma), ""); args[0].([]string)[0] != "acc_me" {
		t.Fatalf("AsSystem 后应按系统账号看，得到 %v", args)
	}
	// 没声明身份：宁可查不到
	if clause, _ := Clause(context.Background(), ""); clause != "1 = 0" {
		t.Fatalf("没声明身份应返回永假条件，得到 %s", clause)
	}
}

func TestNoSystemAccountSeesOnlyFamily(t *testing.T) {
	withSystem(t)
	if clause, args := Clause(AsSystem(context.Background()), ""); clause != "visibility = 'family'" || args != nil {
		t.Fatalf("没有系统账号时只能看全家可见的，得到 %s %v", clause, args)
	}
	// 没有归属账号时新记录只能全家可见，否则谁也看不到
	if Resolve(AsSystem(context.Background()), "") != Family {
		t.Fatal("没有归属账号时应全家可见")
	}
}

func TestResolveAndChange(t *testing.T) {
	withSystem(t, "acc_me")
	me := With(context.Background(), Viewer{Account: "acc_me"})
	grandma := With(context.Background(), Viewer{Account: "acc_nai", Locked: true})
	none := context.Background()

	if Resolve(me, "") != Private || Resolve(me, "family") != Family || Resolve(me, "whatever") != Private {
		t.Fatal("默认私有，明确要 family 才全家可见")
	}
	if Resolve(grandma, "family") != Private {
		t.Fatal("奶奶的账号只能私有")
	}
	if !CanChange(me, "acc_me") || CanChange(me, "acc_nai") || CanChange(grandma, "acc_nai") || CanChange(none, "acc_me") {
		t.Fatal("只有归属人能改，锁定账号、没声明身份不能改")
	}
	if !CanSee(grandma, "acc_me", Family) || CanSee(grandma, "acc_me", Private) || !CanSee(grandma, "acc_nai", Private) || CanSee(none, "acc_me", Family) {
		t.Fatal("可见判断不对")
	}
	if !IsSystem(me) || IsSystem(grandma) || !IsSystem(AsSystem(none)) || IsSystem(none) {
		t.Fatal("IsSystem 判断不对")
	}
	if Owner(grandma) != "acc_nai" || Owner(AsSystem(none)) != "acc_me" || Owner(none) != "" {
		t.Fatal("新记录归属不对")
	}
}
