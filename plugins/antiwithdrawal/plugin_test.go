package antiwithdrawal

import (
	"slices"
	"testing"
	"time"
)

func TestNewPluginMeta(t *testing.T) {
	p := NewPlugin()
	if p.Name == "" || p.Author == "" || p.Version == "" {
		t.Fatalf("插件元信息不完整: %+v", p.Meta)
	}
	if len(p.Platforms) != 1 || p.Platforms[0] != "qq" {
		t.Fatalf("防撤回仅支持 QQ 平台，实际 platforms=%v", p.Platforms)
	}
}

func TestMessageQueue(t *testing.T) {
	q := NewMessageQueue[int](3)
	if got := q.GetAll(); len(got) != 0 {
		t.Fatalf("空队列 GetAll 应返回空，实际 %v", got)
	}
	if got := q.Get(5); len(got) != 0 {
		t.Fatalf("空队列 Get 应返回空，实际 %v", got)
	}

	for i := 1; i <= 3; i++ {
		q.Add(i)
	}
	if got := q.GetAll(); !slices.Equal(got, []int{1, 2, 3}) {
		t.Fatalf("填满后 GetAll = %v，期望 [1 2 3]", got)
	}
	if got := q.GetCount(); got != 3 {
		t.Fatalf("GetCount = %d，期望 3", got)
	}

	// 继续写入触发环形覆盖，最旧的 1、2 被淘汰
	q.Add(4)
	q.Add(5)
	if got := q.GetAll(); !slices.Equal(got, []int{3, 4, 5}) {
		t.Fatalf("覆盖后 GetAll = %v，期望 [3 4 5]", got)
	}
	if got := q.Get(2); !slices.Equal(got, []int{4, 5}) {
		t.Fatalf("Get(2) = %v，期望取最近两条 [4 5]", got)
	}
	if got := q.Get(10); !slices.Equal(got, []int{3, 4, 5}) {
		t.Fatalf("Get(10) = %v，期望返回全部 [3 4 5]", got)
	}
	if got := q.Get(0); len(got) != 0 {
		t.Fatalf("Get(0) 应返回空，实际 %v", got)
	}
}

func TestIsTimeout(t *testing.T) {
	now := uint(time.Now().Unix())
	cases := []struct {
		name string
		ts   uint
		want bool
	}{
		{"刚收到", now, false},
		{"未满3分钟", now - ResourceTimeout + 1, false},
		{"恰好3分钟", now - ResourceTimeout, false},
		{"超过3分钟", now - ResourceTimeout - 1, true},
		{"未来时间戳（时钟偏差）", now + 1000, false},
	}
	for _, c := range cases {
		if got := isTimeout(c.ts); got != c.want {
			t.Errorf("%s: isTimeout(%d) = %v，期望 %v", c.name, c.ts, got, c.want)
		}
	}
}
