package groupdigest

import (
	"testing"
	"time"

	"github.com/jeanhua/AniaBot/common/model/message"
)

func TestNormalizeGroupIDs(t *testing.T) {
	set := normalizeGroupIDs([]string{"123456", "qq:789", " fs:oc_abc ", "", "tg:-100123"})
	for _, want := range []string{"qq:123456", "qq:789", "fs:oc_abc", "tg:-100123"} {
		if _, ok := set[want]; !ok {
			t.Errorf("缺少群 ID %s（实际 %v）", want, set)
		}
	}
	if len(set) != 4 {
		t.Errorf("期望 4 个群，实际 %d", len(set))
	}
}

func TestGroupStateTrigger(t *testing.T) {
	st := &groupState{}
	for i := 0; i < 99; i++ {
		st.add(digestMessage{Text: "x"}, 200)
	}
	if got := st.tryTrigger(100, 0, time.Now()); got != nil {
		t.Fatalf("未达阈值不应触发")
	}
	st.add(digestMessage{Text: "第100条"}, 200)
	got := st.tryTrigger(100, 0, time.Now())
	if len(got) != 100 {
		t.Fatalf("达到阈值应领取 100 条，实际 %d", len(got))
	}
	if st.count != 0 {
		t.Errorf("触发后计数应重置，实际 %d", st.count)
	}
	// 生成中不重复触发
	if got := st.tryTrigger(1, 0, time.Now()); got != nil {
		t.Errorf("生成中不应重复触发")
	}
	st.finish(time.Now())
	// 冷却期内不触发
	st.add(digestMessage{Text: "y"}, 200)
	if got := st.tryTrigger(1, 10*time.Minute, time.Now()); got != nil {
		t.Errorf("冷却期内不应触发")
	}
	// 冷却结束后触发
	if got := st.tryTrigger(1, 10*time.Minute, time.Now().Add(11*time.Minute)); len(got) != 1 {
		t.Errorf("冷却结束后应触发并领取 1 条，实际 %d", len(got))
	}
}

func TestRenderMessageText(t *testing.T) {
	msg := message.Message{
		Sender: message.MessageSender{UserId: message.FromString("10001")},
		Message: []message.OB11Segment{
			{Type: message.SegmentText, Data: map[string]any{"text": "你好"}},
			{Type: message.SegmentImage, Data: map[string]any{"file": "base64://aGk=", "url": "base64://aGk="}},
			{Type: message.SegmentMention, Data: map[string]any{"qq": "all"}},
		},
	}
	got := renderMessageText(msg)
	if got != "你好[图片][at:全体成员]" {
		t.Errorf("渲染结果不符: %q", got)
	}
}
