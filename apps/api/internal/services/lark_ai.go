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
	larkAIInputMaxRunes      = 300
)

var errDeepSeekIntent = errors.New("deepseek intent parsing failed")

const deepSeekIntentPrompt = `你是库存机器人的查询计划生成器。只输出一个 JSON 对象，不要回答用户问题。
允许的 intent：inventory、low_stock、out_of_stock、inventory_summary、movements、today_changes、analytics、help、unknown。
inventory 用于查询某个商品的库存、当前采购价、库存金额或状态，必须提取 keyword。
low_stock 用于查询低库存或补货预警，可选 keyword；out_of_stock 用于列出哪些商品缺货或库存为零，可选 keyword。
inventory_summary 只用于同时查看多项库存概览，或商品种类数、低库存/缺货种类数；单项库存数量或金额统计用 analytics，询问“哪些缺货”用 out_of_stock。
movements 只用于列出最近流水或操作记录，可选 keyword；today_changes 只用于同时查看今天入库、出库和调整的概览，单项数量/笔数用 analytics。
help 用于询问机器人会什么或怎么用；非库存相关问题返回 unknown。只有 inventory、low_stock、out_of_stock、movements 可以包含 keyword。

统计、排行、比较、按条件求总量一律使用 analytics，并组合以下字段：
- metric：inventory_quantity、inventory_value、movement_quantity、movement_count、movement_value。
- group_by：none、product、shop、operator。必须保留用户问的维度，问店铺就用 shop，问商品就用 product，问谁就用 operator。
- movement_type：all、inbound、sales_outbound、adjustment；inventory_* 指标留空。
- time_range：current、today、yesterday、last_n_days、current_month、all。流水没有时间词用 all；inventory_* 只用 current。
- days：仅 last_n_days 使用，1 到 365；sort：分组时 asc 或 desc；limit：分组时 1 到 5。
- product_keyword、shop_keyword、operator_keyword：只保留用户明确给出的名称或编码。
当前库存指标只允许 group_by 为 none 或 product；其余统计使用 movement_* 指标。

示例：
“哪个店铺出货最多” => {"intent":"analytics","metric":"movement_quantity","group_by":"shop","movement_type":"sales_outbound","time_range":"all","sort":"desc","limit":1}
“今天哪个商品卖得最好” => {"intent":"analytics","metric":"movement_quantity","group_by":"product","movement_type":"sales_outbound","time_range":"today","sort":"desc","limit":1}
“近7天张三出库多少” => {"intent":"analytics","metric":"movement_quantity","group_by":"none","movement_type":"sales_outbound","time_range":"last_n_days","days":7,"operator_keyword":"张三"}
“库存金额最高的商品” => {"intent":"analytics","metric":"inventory_value","group_by":"product","time_range":"current","sort":"desc","limit":5}
不要把不支持的维度替换成相近维度，不得生成 SQL。`

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
	input = strings.TrimSpace(input)
	if input == "" {
		return larkCommand{}, errDeepSeekIntent
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
		MaxTokens: 220,
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
		return larkCommand{}, errDeepSeekIntent
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return larkCommand{}, errDeepSeekIntent
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return larkCommand{}, errDeepSeekIntent
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return larkCommand{}, errDeepSeekIntent
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
		return larkCommand{}, errDeepSeekIntent
	}
	choice := completion.Choices[0]
	if choice.FinishReason != "stop" || strings.TrimSpace(choice.Message.Content) == "" {
		return larkCommand{}, errDeepSeekIntent
	}
	return deepSeekResultCommand(choice.Message.Content)
}

func deepSeekResultCommand(content string) (larkCommand, error) {
	var result struct {
		Intent          string `json:"intent"`
		Keyword         string `json:"keyword"`
		Metric          string `json:"metric"`
		GroupBy         string `json:"group_by"`
		MovementType    string `json:"movement_type"`
		TimeRange       string `json:"time_range"`
		Days            int    `json:"days"`
		Sort            string `json:"sort"`
		Limit           int    `json:"limit"`
		ProductKeyword  string `json:"product_keyword"`
		ShopKeyword     string `json:"shop_keyword"`
		OperatorKeyword string `json:"operator_keyword"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return larkCommand{}, errDeepSeekIntent
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return larkCommand{}, errDeepSeekIntent
	}

	result.Intent = strings.TrimSpace(result.Intent)
	result.Keyword = strings.TrimSpace(result.Keyword)
	if len([]rune(result.Keyword)) > larkKeywordMaxRunes ||
		len([]rune(result.ProductKeyword)) > larkKeywordMaxRunes ||
		len([]rune(result.ShopKeyword)) > larkKeywordMaxRunes ||
		len([]rune(result.OperatorKeyword)) > larkKeywordMaxRunes {
		return larkCommand{}, errDeepSeekIntent
	}
	hasAnalyticsFields := result.Metric != "" || result.GroupBy != "" || result.MovementType != "" ||
		result.TimeRange != "" || result.Days != 0 || result.Sort != "" || result.Limit != 0 ||
		result.ProductKeyword != "" || result.ShopKeyword != "" || result.OperatorKeyword != ""
	if result.Intent != "analytics" && hasAnalyticsFields {
		return larkCommand{}, errDeepSeekIntent
	}

	switch result.Intent {
	case "inventory":
		if result.Keyword == "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionInventory, Name: "自然语言·查库存", Keyword: result.Keyword}, nil
	case "low_stock":
		return larkCommand{Action: larkActionLowStock, Name: "自然语言·低库存", Keyword: result.Keyword}, nil
	case "out_of_stock":
		return larkCommand{Action: larkActionOutOfStock, Name: "自然语言·缺货清单", Keyword: result.Keyword}, nil
	case "inventory_summary":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionSummary, Name: "自然语言·库存概览"}, nil
	case "movements":
		return larkCommand{Action: larkActionMovements, Name: "自然语言·查流水", Keyword: result.Keyword}, nil
	case "today_changes":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionTodayChanges, Name: "自然语言·今日变动"}, nil
	case "analytics":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		plan, err := normalizeLarkAnalyticsPlan(larkAnalyticsPlan{
			Metric:          result.Metric,
			GroupBy:         result.GroupBy,
			MovementType:    result.MovementType,
			TimeRange:       result.TimeRange,
			Days:            result.Days,
			Sort:            result.Sort,
			Limit:           result.Limit,
			ProductKeyword:  result.ProductKeyword,
			ShopKeyword:     result.ShopKeyword,
			OperatorKeyword: result.OperatorKeyword,
		})
		if err != nil {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionAnalytics, Name: "自然语言·统计查询", Analytics: &plan}, nil
	case "help":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionHelp, Name: "自然语言·帮助"}, nil
	case "unknown":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionUnknown, Name: "自然语言·未识别"}, nil
	default:
		return larkCommand{}, fmt.Errorf("%w: unsupported intent", errDeepSeekIntent)
	}
}
