package service

// 本文件是 AI 应答的检索内核（RAG 的 R）：纯 Go 打分，不引入向量库 / 外部检索服务。
//
// 检索语料 = 「渠道的自动回复规则」。不再维护独立知识库表：客服/管理员在后台「自动回复」
// 里维护的关键词 + 回复内容，既是精确命中规则，也是 AI 应答时的检索片段，一处维护、两处生效。
//
// 打分口径（归一化到 0~1，便于和后台的 rag.min_score 直接比较）：
//   1. 中文按 2-gram 切分（退货怎么弄 → 退货 / 货怎 / 怎么 / 么弄），英文/数字按词切分；
//   2. 另算一份单字覆盖率：中文口语里同义改写很多（退货/退款/退换），单字层面仍能对上；
//   3. 覆盖率按「字段权重」折算：命中规则名/关键词比只命中回复正文更相关；
//   4. 关键词命中额外加权：规则的关键词就是「这条规则解决什么问题」的人工标注。
//
// 形态约定：检索能力是 RagService 的方法（指针接收者）；打分纯函数单独抽出来便于单测。

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"kefu-server/internal/common/model"
	"kefu-server/internal/common/repository"
)

const (
	// ragScanLimit 单租户参与打分的规则上限：自动回复规则是人工维护的小表，
	// 500 条已远超正常规模，加上限只是防止误导入上万条数据后把同步回复拖慢。
	ragScanLimit = 500
	// 字段权重：同样的词命中规则名 / 关键词 / 回复正文，可信度不同。
	ragWeightTitle   = 1.0
	ragWeightTags    = 1.0
	ragWeightContent = 0.85
	// 打分公式的各项系数（三者相加 = 1，保证结果落在 0~1）。
	ragWeightBigram   = 0.55
	ragWeightUnigram  = 0.30
	ragWeightTagBonus = 0.15
	// ragMinTokenLen 英文/数字词的最短长度：单字母词（a / i）区分度太低，不参与打分。
	ragMinTokenLen = 2
)

// RagService AI 应答检索内核，持有自动回复仓储。
type RagService struct {
	repo *repository.AutoReplyRepository
}

// NewRagService 创建检索实例（自带数据访问层依赖）。
func NewRagService() *RagService {
	return &RagService{repo: repository.NewAutoReplyRepository()}
}

// Retrieve 检索与问题最相关的「该渠道自动回复规则」，返回命中的规则与各自分数（同序、按分数降序）。
//
// 契约：
//   - 只检索启用中的规则，且规则适用于 channelID（含全渠道规则）；
//   - 分数低于 minScore 的规则一律丢弃：宁可「无命中」走兜底话术，也不拿弱相关内容去猜；
//   - topK <= 0 时按 1 处理，超过 ragScanLimit 时按上限夹取；
//   - 无命中返回 (空切片, 空切片, nil)，不返回 error（无命中不是错误）。
func (s *RagService) Retrieve(tenantID, channelID uint, question string, topK int, minScore float64) ([]*model.AutoReply, []float64, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return []*model.AutoReply{}, []float64{}, nil
	}
	if topK <= 0 {
		topK = 1
	}
	if topK > ragScanLimit {
		topK = ragScanLimit
	}
	if minScore < 0 {
		minScore = 0
	}
	if minScore > 1 {
		minScore = 1
	}

	rules, err := s.repo.ListEnabledAutoReplies(tenantID)
	if err != nil {
		return nil, nil, err
	}

	type hit struct {
		rule  *model.AutoReply
		score float64
	}
	hits := make([]hit, 0, len(rules))
	for _, r := range rules {
		if r == nil || r.TransferToHuman {
			continue // 转人工规则不参与 AI 检索
		}
		if !MatchChannel(r, channelID) {
			continue // 只检索适用于该渠道的规则
		}
		if score := scoreAutoReply(q, r); score >= minScore && score > 0 {
			hits = append(hits, hit{rule: r, score: score})
		}
	}
	// 分数降序；同分按 id 升序（结果稳定，便于后台复现与单测）
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].rule.ID < hits[j].rule.ID
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}

	outRules := make([]*model.AutoReply, 0, len(hits))
	outScores := make([]float64, 0, len(hits))
	for _, h := range hits {
		outRules = append(outRules, h.rule)
		outScores = append(outScores, h.score)
	}
	return outRules, outScores, nil
}

// ---------------- 打分（纯函数，便于单测） ----------------

// scoreAutoReply 计算一条自动回复规则与问题的相关度（0~1）。
// 纯函数：不碰数据库、不读设置，同样的入参永远得到同样的分数。
//
// 字段映射：
//   - title   = 规则名 + 关键词（命中权重最高）；
//   - tags    = 关键词（后台用关键词标出来的就是「这条规则解决什么问题」）；
//   - content = 回复正文。
func scoreAutoReply(question string, rule *model.AutoReply) float64 {
	if rule == nil {
		return 0
	}
	bigrams, unigrams := splitQueryTokens(question)
	if len(bigrams) == 0 && len(unigrams) == 0 {
		return 0
	}
	title := strings.TrimSpace(rule.Name)
	if title == "" {
		title = strings.TrimSpace(rule.Keywords)
	}
	tags := strings.TrimSpace(rule.Keywords)

	titleBigram, titleUnigram := indexText(title)
	tagBigram, tagUnigram := indexText(tags)
	bodyBigram, bodyUnigram := indexText(rule.Reply)

	bigramCov := coverage(bigrams, []fieldIndex{
		{set: titleBigram, weight: ragWeightTitle},
		{set: tagBigram, weight: ragWeightTags},
		{set: bodyBigram, weight: ragWeightContent},
	})
	unigramCov := coverage(unigrams, []fieldIndex{
		{set: titleUnigram, weight: ragWeightTitle},
		{set: tagUnigram, weight: ragWeightTags},
		{set: bodyUnigram, weight: ragWeightContent},
	})

	score := ragWeightBigram*bigramCov + ragWeightUnigram*unigramCov +
		ragWeightTagBonus*tagHitRatio(question, tags)

	return clamp01(score)
}

// fieldIndex 一份字段的词集合与命中权重。
type fieldIndex struct {
	set    map[string]bool
	weight float64
}

// coverage 问题词的覆盖率：每个词取「命中字段里最高的权重」，取平均后得到 0~1。
// 用「最高权重」而不是累加，避免一条把关键词重复十遍的正文把分数刷满。
func coverage(tokens []string, fields []fieldIndex) float64 {
	if len(tokens) == 0 {
		return 0
	}
	var sum float64
	for _, tk := range tokens {
		best := 0.0
		for _, f := range fields {
			if f.set[tk] && f.weight > best {
				best = f.weight
			}
		}
		sum += best
	}
	return clamp01(sum / float64(len(tokens)))
}

// tagHitRatio 关键词命中率：问题里直接出现该关键词时说明指向性很强
// （如关键词「退货」+ 问题「退货怎么弄」）。
func tagHitRatio(question, tags string) float64 {
	tagList := splitTagList(tags)
	if len(tagList) == 0 {
		return 0
	}
	q := strings.ToLower(question)
	hit := 0
	for _, tag := range tagList {
		if tag != "" && strings.Contains(q, tag) {
			hit++
		}
	}
	return clamp01(float64(hit) / float64(len(tagList)))
}

// splitTagList 把逗号分隔的关键词切成小写片段（兼容中文逗号 / 分号，与关键词配置同一口径）。
func splitTagList(tags string) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	// splitKeywords（match.go）已统一处理半角/全角逗号、分号与换行
	for _, seg := range splitKeywords(tags) {
		seg = strings.ToLower(strings.TrimSpace(seg))
		if seg == "" || seen[seg] {
			continue
		}
		seen[seg] = true
		out = append(out, seg)
	}
	return out
}

// splitQueryTokens 把问题切成「2-gram 集合」与「单字集合」：
//   - 英文 / 数字按连续字母数字切成整词（refund / 48），长度 < ragMinTokenLen 的丢弃；
//   - 中文（含其它非 ASCII 字母）按连续的「汉字段」切 2-gram，长度 1 的段同时贡献单字；
//   - 单字集合包含所有汉字（含 2-gram 段的每个字），用于兜同义改写。
func splitQueryTokens(question string) (bigrams []string, unigrams []string) {
	q := strings.ToLower(strings.TrimSpace(question))
	if q == "" {
		return nil, nil
	}
	bg := map[string]bool{}
	uni := map[string]bool{}

	runes := []rune(q)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case isASCIIWordRune(r):
			j := i
			for j < len(runes) && isASCIIWordRune(runes[j]) {
				j++
			}
			if w := string(runes[i:j]); len([]rune(w)) >= ragMinTokenLen {
				bg[w] = true
			}
			i = j
		case isCJK(r):
			j := i
			for j < len(runes) && isCJK(runes[j]) {
				j++
			}
			seg := runes[i:j]
			for k := range seg {
				uni[string(seg[k])] = true
				if k+1 < len(seg) {
					bg[string(seg[k:k+2])] = true
				}
			}
			i = j
		default:
			// 标点 / 空白 / emoji 一律当分隔符
			i++
		}
	}
	return sortedKeys(bg), sortedKeys(uni)
}

// indexText 把一段文本建成「2-gram 集合 + 单字集合」，供 coverage 命中判断。
func indexText(text string) (map[string]bool, map[string]bool) {
	bigrams, unigrams := splitQueryTokens(text)
	return toSet(bigrams), toSet(unigrams)
}

// isASCIIWordRune 判断是否为英文 / 数字词内字符（词内连接符也当分隔符处理）。
func isASCIIWordRune(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// isCJK 判断是否为中日韩统一表意文字。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// toSet 把切片转成集合。
func toSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}
	return out
}

// sortedKeys 返回集合的键，排序后输出（保证打分与单测结果稳定）。
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// clamp01 把结果夹到 0~1。
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ---------------- 提示词拼装 ----------------

// BuildRagPrompt 把命中的自动回复规则拼进提示词，附带来源标题，便于模型引用与人工核对。
// 输出形如：
//
//	【参考资料】
//	[1] 退货政策（相关度 0.42）
//	7 天无理由退货：...
//
//	【客户问题】
//	退货怎么弄
//
//	【回答要求】
//	只依据上面的参考资料回答；资料没有覆盖的信息不要编造，直接说明需要转接人工客服。
func BuildRagPrompt(question string, rules []*model.AutoReply, scores []float64) string {
	var b strings.Builder
	b.WriteString("【参考资料】\n")
	for i, rule := range rules {
		if rule == nil {
			continue
		}
		score := 0.0
		if i < len(scores) {
			score = scores[i]
		}
		b.WriteString("[")
		b.WriteString(itoa(i + 1))
		b.WriteString("] ")
		title := strings.TrimSpace(rule.Name)
		if title == "" {
			title = firstKeyword(rule.Keywords)
		}
		if title == "" {
			title = "自动回复规则"
		}
		b.WriteString(title)
		b.WriteString("（相关度 ")
		b.WriteString(formatScore(score))
		b.WriteString("）\n")
		b.WriteString(strings.TrimSpace(rule.Reply))
		b.WriteString("\n\n")
	}
	b.WriteString("【客户问题】\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n【回答要求】\n只依据上面的参考资料回答；资料没有覆盖的信息不要编造，直接说明需要转接人工客服。\n")
	return b.String()
}

// firstKeyword 取规则关键词里的第一个（用于没有规则名时的标题兜底）。
func firstKeyword(keywords string) string {
	for _, kw := range splitKeywords(keywords) {
		if kw = strings.TrimSpace(kw); kw != "" {
			return kw
		}
	}
	return ""
}

// itoa 提示词里的序号（避免为一个数字引入 fmt 依赖）。
func itoa(n int) string { return strconv.Itoa(n) }

// formatScore 相关度保留两位小数，便于人工核对检索质量。
func formatScore(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
