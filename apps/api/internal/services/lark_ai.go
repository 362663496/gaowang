package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	deepSeekEndpoint         = "https://api.deepseek.com/chat/completions"
	deepSeekResponseMaxBytes = 32 << 10
	deepSeekTotalTimeout     = 10 * time.Second
	deepSeekRetryDelay       = 100 * time.Millisecond
	larkAIInputMaxRunes      = 300
)

var errDeepSeekIntent = errors.New("deepseek intent parsing failed")

type deepSeekIntentError struct {
	Stage      string
	StatusCode int
	Retryable  bool
	Attempts   int
	Elapsed    time.Duration
}

func (e *deepSeekIntentError) Error() string {
	return fmt.Sprintf("%s: stage=%s status=%d retryable=%t attempts=%d elapsed_ms=%d",
		errDeepSeekIntent, e.Stage, e.StatusCode, e.Retryable, e.Attempts, e.Elapsed.Milliseconds())
}

func (e *deepSeekIntentError) Unwrap() error { return errDeepSeekIntent }

func deepSeekFailure(stage string, statusCode int, retryable bool) error {
	return &deepSeekIntentError{Stage: stage, StatusCode: statusCode, Retryable: retryable}
}

func finalizeDeepSeekFailure(err error, attempts int, elapsed time.Duration) error {
	var diagnostic *deepSeekIntentError
	if !errors.As(err, &diagnostic) {
		return &deepSeekIntentError{Stage: "plan_validate", Attempts: attempts, Elapsed: elapsed}
	}
	copy := *diagnostic
	copy.Attempts = attempts
	copy.Elapsed = elapsed
	return &copy
}

func deepSeekFailureIsPlan(err error) bool {
	var diagnostic *deepSeekIntentError
	return errors.As(err, &diagnostic) && (diagnostic.Stage == "plan_decode" || diagnostic.Stage == "plan_validate")
}

const deepSeekIntentPrompt = `你是库存业务只读查询计划生成器。只输出一个 JSON 对象，不回答问题、不计算数字、不生成 SQL、Markdown 或飞书卡片。

简单 intent：inventory、low_stock、out_of_stock、inventory_summary、movements、today_changes、analytics、help、unknown。
- inventory 查询单个商品当前库存/采购价/库存价值，必须有 keyword。
- low_stock、out_of_stock、movements 可有 keyword；其他简单 intent 不得有 keyword。
- 所有统计、排行、宽范围明细、趋势、占比、比较和多维问题使用 analytics。

analytics 可查询字段：
- domain：product、inventory、shop、movement、operator。
- metric：inventory_quantity、inventory_value、movement_quantity、movement_count、movement_value。movement_value 是按当前采购价估算的采购/出库成本，不是销售额。
- operation：total、details、ranking、trend、share、comparison。
- group_by：数组，元素只能是 product、shop、operator，最多两个且不得重复；必须保留用户要求的全部维度。
- movement_type：all、inbound、sales_outbound、adjustment。
- time_range：current、today、yesterday、current_week、previous_week、current_month、previous_month、current_year、last_n_days、date_range、all。
- days：仅 last_n_days，1..365；date_from/date_to：仅 date_range，YYYY-MM-DD，含首尾日期。
- time_bucket：trend 时 day 或 month；compare_to：comparison 时 previous_period 或 previous_year。
- sort：asc 或 desc；limit：1..10。
- presentation：auto、summary、detail、ranking、matrix、trend、share、comparison。
- product_keyword、shop_keyword、operator_keyword：只放用户明确给出的名称或编码。

规则：
- inventory_* 只查询 current，只能 total/ranking/share，分组只能无或 [product]。
- movement_* 可按商品、店铺、操作人任意一维或二维聚合。
- details 只列逐条原始记录，metric 必须为空且 group_by 必须为 []，最多 10 条。
- 用户问“哪些/每个/各”商品、店铺或操作人“多少数量/几笔/金额”时属于按维度聚合，使用 ranking 和对应 metric/group_by，不使用 details。
- ranking 一到二维；share 仅一维；trend 最多再带一个业务维度；comparison 最多一个维度。
- 三维及以上、敏感账号/权限/备份、写操作或能力外问题返回 unknown，不得静默丢维度。
- 数据只限商品、当前库存、店铺、出入库流水和历史操作人姓名；不得查询邮箱、角色、权限、密码、审计、备份或系统设置。
- presentation=auto 时由服务端选择；operation/presentation 标签冲突时服务端按 metric、group_by 和真实结果纠正，不丢筛选、维度或时间条件。

示例：
“哪个店铺出货最多” => {"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"ranking","group_by":["shop"],"movement_type":"sales_outbound","time_range":"all","sort":"desc","limit":1,"presentation":"ranking"}
“今天出库了哪些商品，多少数量” => {"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"ranking","group_by":["product"],"movement_type":"sales_outbound","time_range":"today","sort":"desc","limit":10,"presentation":"ranking"}
“今天按商品排序并列出店铺各占多少” => {"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"ranking","group_by":["product","shop"],"movement_type":"sales_outbound","time_range":"today","sort":"desc","limit":10,"presentation":"matrix"}
“本月各店铺销量占比” => {"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"share","group_by":["shop"],"movement_type":"sales_outbound","time_range":"current_month","limit":10,"presentation":"share"}
“今年每月出库趋势” => {"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"trend","group_by":[],"movement_type":"sales_outbound","time_range":"current_year","time_bucket":"month","limit":10,"presentation":"trend"}
“7月1日到7月31日和上期相比各商品出库量” => {"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"comparison","group_by":["product"],"movement_type":"sales_outbound","time_range":"date_range","date_from":"2026-07-01","date_to":"2026-07-31","compare_to":"previous_period","limit":10,"presentation":"comparison"}`

const deepSeekPlanRepairPrompt = `上一个 JSON 未通过本地计划校验。保留原问题的筛选、维度和时间，只使用系统消息列出的字段与枚举纠正计划；仍只输出一个 JSON 对象。超出商品业务数据边界时输出 {"intent":"unknown"}。`

type deepSeekIntentParser struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

func newDeepSeekIntentParser(apiKey string, model string) *deepSeekIntentParser {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &deepSeekIntentParser{
		apiKey:   apiKey,
		model:    model,
		endpoint: deepSeekEndpoint,
		client:   &http.Client{Timeout: 6 * time.Second},
	}
}

func (p *deepSeekIntentParser) Parse(ctx context.Context, input string) (larkCommand, error) {
	started := time.Now()
	input = strings.TrimSpace(input)
	if input == "" {
		return larkCommand{}, finalizeDeepSeekFailure(deepSeekFailure("plan_validate", 0, false), 0, time.Since(started))
	}
	inputRunes := []rune(input)
	if len(inputRunes) > larkAIInputMaxRunes {
		input = string(inputRunes[:larkAIInputMaxRunes])
	}

	payload := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Thinking struct {
			Type string `json:"type"`
		} `json:"thinking"`
		ResponseFormat struct {
			Type string `json:"type"`
		} `json:"response_format"`
		MaxTokens int `json:"max_tokens"`
	}{
		Model:     p.model,
		MaxTokens: 480,
	}
	payload.Messages = append(payload.Messages,
		struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: "system", Content: deepSeekIntentPrompt},
		struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: "user", Content: input},
	)
	payload.Thinking.Type = "disabled"
	payload.ResponseFormat.Type = "json_object"

	body, err := json.Marshal(payload)
	if err != nil {
		return larkCommand{}, finalizeDeepSeekFailure(deepSeekFailure("marshal", 0, false), 0, time.Since(started))
	}
	parseCtx, cancel := context.WithTimeout(ctx, deepSeekTotalTimeout)
	defer cancel()

	var retryDiagnostic *deepSeekIntentError
	for attempt := 1; attempt <= 2; attempt++ {
		content, attemptErr := p.attempt(parseCtx, body)
		if attemptErr == nil {
			command, planErr := deepSeekResultCommand(content)
			if planErr != nil {
				var diagnostic *deepSeekIntentError
				if attempt == 1 && errors.As(planErr, &diagnostic) && diagnostic.Stage == "plan_validate" {
					retryDiagnostic = diagnostic
					payload.Messages = append(payload.Messages,
						struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{Role: "assistant", Content: content},
						struct {
							Role    string `json:"role"`
							Content string `json:"content"`
						}{Role: "user", Content: deepSeekPlanRepairPrompt},
					)
					body, err = json.Marshal(payload)
					if err != nil {
						return larkCommand{}, finalizeDeepSeekFailure(deepSeekFailure("marshal", 0, false), attempt, time.Since(started))
					}
					continue
				}
				return larkCommand{}, finalizeDeepSeekFailure(planErr, attempt, time.Since(started))
			}
			command.AIAttempts = attempt
			command.AIElapsedMS = time.Since(started).Milliseconds()
			if retryDiagnostic != nil {
				command.AIStage = retryDiagnostic.Stage
				command.AIStatus = retryDiagnostic.StatusCode
			}
			return command, nil
		}
		var diagnostic *deepSeekIntentError
		if !errors.As(attemptErr, &diagnostic) || !diagnostic.Retryable || attempt == 2 {
			return larkCommand{}, finalizeDeepSeekFailure(attemptErr, attempt, time.Since(started))
		}
		retryDiagnostic = diagnostic
		timer := time.NewTimer(deepSeekRetryDelay)
		select {
		case <-parseCtx.Done():
			timer.Stop()
			return larkCommand{}, finalizeDeepSeekFailure(attemptErr, attempt, time.Since(started))
		case <-timer.C:
		}
	}
	return larkCommand{}, finalizeDeepSeekFailure(deepSeekFailure("transport", 0, true), 2, time.Since(started))
}

func (p *deepSeekIntentParser) attempt(ctx context.Context, body []byte) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", deepSeekFailure("request", 0, false)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return "", deepSeekFailure("transport", 0, true)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError
		return "", deepSeekFailure("http_status", response.StatusCode, retryable)
	}

	var completion struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, deepSeekResponseMaxBytes))
	if err := decoder.Decode(&completion); err != nil || len(completion.Choices) == 0 {
		return "", deepSeekFailure("response_decode", 0, false)
	}
	choice := completion.Choices[0]
	if choice.FinishReason != "stop" || strings.TrimSpace(choice.Message.Content) == "" {
		return "", deepSeekFailure("finish_reason", 0, false)
	}
	return choice.Message.Content, nil
}

type deepSeekGroupBy []string

func (group *deepSeekGroupBy) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*group = nil
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*group = values
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value == "" || value == larkGroupNone {
		*group = nil
	} else {
		*group = []string{value}
	}
	return nil
}

func deepSeekResultCommand(content string) (larkCommand, error) {
	var result struct {
		Intent          string          `json:"intent"`
		Keyword         string          `json:"keyword"`
		Domain          string          `json:"domain"`
		Metric          string          `json:"metric"`
		Operation       string          `json:"operation"`
		GroupBy         deepSeekGroupBy `json:"group_by"`
		MovementType    string          `json:"movement_type"`
		TimeRange       string          `json:"time_range"`
		Days            int             `json:"days"`
		DateFrom        string          `json:"date_from"`
		DateTo          string          `json:"date_to"`
		TimeBucket      string          `json:"time_bucket"`
		CompareTo       string          `json:"compare_to"`
		Sort            string          `json:"sort"`
		Limit           int             `json:"limit"`
		Presentation    string          `json:"presentation"`
		ProductKeyword  string          `json:"product_keyword"`
		ShopKeyword     string          `json:"shop_keyword"`
		OperatorKeyword string          `json:"operator_keyword"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return larkCommand{}, deepSeekFailure("plan_decode", 0, false)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return larkCommand{}, deepSeekFailure("plan_decode", 0, false)
	}

	result.Intent = strings.TrimSpace(result.Intent)
	result.Keyword = strings.TrimSpace(result.Keyword)
	if len([]rune(result.Keyword)) > larkKeywordMaxRunes ||
		len([]rune(result.ProductKeyword)) > larkKeywordMaxRunes ||
		len([]rune(result.ShopKeyword)) > larkKeywordMaxRunes ||
		len([]rune(result.OperatorKeyword)) > larkKeywordMaxRunes {
		return larkCommand{}, deepSeekFailure("plan_validate", 0, false)
	}
	hasAnalyticsFields := result.Domain != "" || result.Metric != "" || result.Operation != "" || len(result.GroupBy) != 0 ||
		result.MovementType != "" || result.TimeRange != "" || result.Days != 0 || result.DateFrom != "" || result.DateTo != "" ||
		result.TimeBucket != "" || result.CompareTo != "" || result.Sort != "" || result.Limit != 0 || result.Presentation != "" ||
		result.ProductKeyword != "" || result.ShopKeyword != "" || result.OperatorKeyword != ""
	if result.Intent != "analytics" && hasAnalyticsFields {
		return larkCommand{}, deepSeekFailure("plan_validate", 0, false)
	}

	simple := func(action string, name string, keywordAllowed bool, keywordRequired bool) (larkCommand, error) {
		if (!keywordAllowed && result.Keyword != "") || (keywordRequired && result.Keyword == "") {
			return larkCommand{}, deepSeekFailure("plan_validate", 0, false)
		}
		return larkCommand{Action: action, Name: name, Keyword: result.Keyword}, nil
	}
	switch result.Intent {
	case "inventory":
		return simple(larkActionInventory, "自然语言·查库存", true, true)
	case "low_stock":
		return simple(larkActionLowStock, "自然语言·低库存", true, false)
	case "out_of_stock":
		return simple(larkActionOutOfStock, "自然语言·缺货清单", true, false)
	case "inventory_summary":
		return simple(larkActionSummary, "自然语言·库存概览", false, false)
	case "movements":
		return simple(larkActionMovements, "自然语言·查流水", true, false)
	case "today_changes":
		return simple(larkActionTodayChanges, "自然语言·今日变动", false, false)
	case "analytics":
		if result.Keyword != "" || len(result.GroupBy) > 2 {
			return larkCommand{}, deepSeekFailure("plan_validate", 0, false)
		}
		plan := larkAnalyticsPlan{
			Domain: result.Domain, Metric: result.Metric, Operation: result.Operation,
			MovementType: result.MovementType, TimeRange: result.TimeRange, Days: result.Days,
			DateFrom: result.DateFrom, DateTo: result.DateTo, TimeBucket: result.TimeBucket,
			CompareTo: result.CompareTo, Sort: result.Sort, Limit: result.Limit, Presentation: result.Presentation,
			ProductKeyword: result.ProductKeyword, ShopKeyword: result.ShopKeyword, OperatorKeyword: result.OperatorKeyword,
		}
		if len(result.GroupBy) > 0 {
			plan.GroupBy = result.GroupBy[0]
		}
		if len(result.GroupBy) > 1 {
			plan.GroupBy2 = result.GroupBy[1]
		}
		plan, err := normalizeLarkAnalyticsPlan(plan)
		if err != nil {
			return larkCommand{}, deepSeekFailure("plan_validate", 0, false)
		}
		return larkCommand{Action: larkActionAnalytics, Name: "自然语言·统计查询", Analytics: &plan}, nil
	case "help":
		return simple(larkActionHelp, "自然语言·帮助", false, false)
	case "unknown":
		return simple(larkActionUnknown, "自然语言·未识别", false, false)
	default:
		return larkCommand{}, deepSeekFailure("plan_validate", 0, false)
	}
}
