package service

// RAG 检索打分（ScoreKnowledgeDoc / splitQueryTokens / BuildRagPrompt）是纯函数，
// 因此用同包测试直接调用：不需要数据库，也不会产生外部依赖。
//
// 验收口径（与后台 rag.min_score 默认值 0.15 对齐）：
//   - 问「退货怎么弄」时，退货知识必须排在发货知识之前，且分数高于阈值；
//   - 与知识库完全无关的问题必须打 0 分（走兜底话术，不调模型）。

import (
	"strings"
	"testing"

	"kefu-server/internal/common/model"
)

// 评测用的两条知识（与验收用例一致）
func ragTestDocs() (*model.KnowledgeDoc, *model.KnowledgeDoc, *model.KnowledgeDoc) {
	refund := &model.KnowledgeDoc{
		Model:   model.Model{ID: 1},
		Title:   "退货政策",
		Tags:    "退货,退款",
		Content: "7 天无理由退货：签收后 7 天内可申请退货，商品不影响二次销售即可，运费由买家承担。",
	}
	shipping := &model.KnowledgeDoc{
		Model:   model.Model{ID: 2},
		Title:   "发货时效",
		Tags:    "发货,物流",
		Content: "48 小时内发货：付款后 48 小时内发出，节假日顺延，物流单号可在「我的订单」查看。",
	}
	warranty := &model.KnowledgeDoc{
		Model:   model.Model{ID: 3},
		Title:   "蓝牙耳机质保",
		Tags:    "质保,保修",
		Content: "蓝牙耳机保修期为一年，非人为损坏可免费换新，需提供订单号。",
	}
	return refund, shipping, warranty
}

// TestScoreKnowledgeDocRanksRefundFirst 「退货怎么弄」必须命中退货那条且分数最高。
func TestScoreKnowledgeDocRanksRefundFirst(t *testing.T) {
	refund, shipping, _ := ragTestDocs()
	q := "退货怎么弄"

	rs := ScoreKnowledgeDoc(q, refund)
	ss := ScoreKnowledgeDoc(q, shipping)

	t.Logf("score(退货怎么弄, 退货政策) = %.4f", rs)
	t.Logf("score(退货怎么弄, 发货时效) = %.4f", ss)

	if rs <= ss {
		t.Fatalf("退货知识得分 %.4f 不高于发货知识 %.4f，排序口径失效", rs, ss)
	}
	if rs < 0.15 {
		t.Fatalf("退货知识得分 %.4f 低于默认阈值 0.15，会被误判为无命中", rs)
	}
	if ss >= 0.15 {
		t.Fatalf("发货知识得分 %.4f 高于阈值 0.15，会产生弱相关噪声", ss)
	}
}

// TestScoreKnowledgeDocUnrelatedBelowThreshold 与知识库无关的问题必须低于默认阈值 0.15，
// 也就是「检索无命中 → 不调模型、直接兜底话术」。
//
// 说明：中文单字覆盖率会带来极小的噪声分（如「今天天气」与「7 天无理由」共享「天」），
// 这是刻意的取舍 —— 用阈值而不是「严格等于 0」来判定无命中，才不会漏掉同义改写的问法。
func TestScoreKnowledgeDocUnrelatedBelowThreshold(t *testing.T) {
	refund, shipping, warranty := ragTestDocs()
	for _, q := range []string{"今天天气怎么样", "你们公司在哪个城市上市了", "asdfghjkl"} {
		for _, doc := range []*model.KnowledgeDoc{refund, shipping, warranty} {
			s := ScoreKnowledgeDoc(q, doc)
			if s >= 0.15 {
				t.Errorf("无关问题 %q 对知识 %q 得分 %.4f，达到阈值 0.15（会误调模型）", q, doc.Title, s)
			}
			t.Logf("无关问题 %q vs %q = %.4f（< 0.15，视为无命中）", q, doc.Title, s)
		}
	}
}

// TestScoreKnowledgeDocWarrantyQuery 口语化提问也要能命中对应知识（AI 链路的实测用例）。
func TestScoreKnowledgeDocWarrantyQuery(t *testing.T) {
	refund, shipping, warranty := ragTestDocs()
	q := "耳机保修多久呀"

	w := ScoreKnowledgeDoc(q, warranty)
	r := ScoreKnowledgeDoc(q, refund)
	s := ScoreKnowledgeDoc(q, shipping)
	t.Logf("score(耳机保修多久呀) = 质保 %.4f / 退货 %.4f / 发货 %.4f", w, r, s)

	if w <= r || w <= s {
		t.Fatalf("质保知识未排第一：质保 %.4f / 退货 %.4f / 发货 %.4f", w, r, s)
	}
	if w < 0.15 {
		t.Fatalf("质保知识得分 %.4f 低于阈值 0.15", w)
	}
}

// TestScoreKnowledgeDocTitleAndTagBoost 同样的词命中标题 / 标签，应高于只命中正文。
func TestScoreKnowledgeDocTitleAndTagBoost(t *testing.T) {
	titleHit := &model.KnowledgeDoc{Title: "发票开具", Tags: "", Content: "下单后可申请电子发票。"}
	bodyHit := &model.KnowledgeDoc{Title: "其他说明", Tags: "", Content: "发票需要联系人工客服补开。"}
	q := "发票"

	th := ScoreKnowledgeDoc(q, titleHit)
	bh := ScoreKnowledgeDoc(q, bodyHit)
	t.Logf("命中标题 = %.4f，命中正文 = %.4f", th, bh)
	if th <= bh {
		t.Fatalf("标题命中得分 %.4f 未高于正文命中 %.4f", th, bh)
	}
}

// TestScoreKnowledgeDocNilAndEmpty 空入参不能 panic，且必须返回 0。
func TestScoreKnowledgeDocNilAndEmpty(t *testing.T) {
	if got := ScoreKnowledgeDoc("退货", nil); got != 0 {
		t.Errorf("nil 知识得分 = %v，期望 0", got)
	}
	doc := &model.KnowledgeDoc{Title: "退货政策"}
	if got := ScoreKnowledgeDoc("", doc); got != 0 {
		t.Errorf("空问题得分 = %v，期望 0", got)
	}
	if got := ScoreKnowledgeDoc("!!!", doc); got != 0 {
		t.Errorf("纯标点问题得分 = %v，期望 0", got)
	}
}

// TestScoreKnowledgeDocBounded 分数必须落在 0~1（后台 min_score 是 0~1 语义）。
func TestScoreKnowledgeDocBounded(t *testing.T) {
	refund, _, _ := ragTestDocs()
	for _, q := range []string{"退货", "退货退货退货退货", "7天无理由退货退款运费", "退货 退款 运费 二次销售 买家"} {
		if s := ScoreKnowledgeDoc(q, refund); s < 0 || s > 1 {
			t.Errorf("问题 %q 得分 %v 越界", q, s)
		}
	}
}

// TestSplitQueryTokens 中文按 2-gram + 单字切分，英文/数字按词切分。
func TestSplitQueryTokens(t *testing.T) {
	bigrams, unigrams := splitQueryTokens("退货48h refund")
	for _, bg := range bigrams {
		t.Logf("bigram: %q", bg)
	}
	if !containsStr(bigrams, "退货") {
		t.Errorf("缺少中文 2-gram「退货」，实际：%v", bigrams)
	}
	if !containsStr(bigrams, "refund") {
		t.Errorf("缺少英文词 refund，实际：%v", bigrams)
	}
	if containsStr(bigrams, "h") {
		t.Errorf("单字母词不应参与打分，实际：%v", bigrams)
	}
	if !containsStr(unigrams, "退") || !containsStr(unigrams, "货") {
		t.Errorf("缺少单字 退 / 货，实际：%v", unigrams)
	}
}

// TestBuildRagPrompt 提示词必须带上知识标题、正文与客户问题（便于核对回答依据）。
func TestBuildRagPrompt(t *testing.T) {
	refund, _, _ := ragTestDocs()
	prompt := BuildRagPrompt("退货怎么弄", []*model.KnowledgeDoc{refund}, []float64{0.42})

	for _, want := range []string{"【知识库片段】", "退货政策", "7 天无理由退货", "相关度 0.42", "【客户问题】", "退货怎么弄"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("提示词缺少片段 %q\n---\n%s", want, prompt)
		}
	}
	t.Logf("提示词：\n%s", prompt)
}

// containsStr 切片是否包含某值。
func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
