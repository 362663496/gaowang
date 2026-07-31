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

const deepSeekIntentPrompt = `你是库存机器人的意图分类器。只输出一个 JSON 对象，不要回答用户问题。
JSON 格式：{"intent":"意图","keyword":"可选商品名称或编码"}
允许的 intent：inventory、low_stock、out_of_stock、inventory_summary、inventory_value_ranking、movements、today_changes、today_sales_ranking、help、unknown。
inventory 用于查询某个商品的库存、当前采购价、库存金额或状态，必须提取 keyword。
low_stock 用于查询低库存或补货预警，可选 keyword；out_of_stock 用于列出哪些商品缺货或库存为零，可选 keyword。
inventory_summary 用于整体商品数、库存数、库存总金额或缺货数量；询问“多少/总计”用它，询问“哪些缺货”用 out_of_stock。
inventory_value_ranking 用于查询哪些商品库存金额最高、库存价值排行；movements 用于最近流水或操作记录，可选 keyword。
today_changes 用于今天入库、出库或调整的总量/笔数；today_sales_ranking 用于今天哪些商品卖得最多或销售排行。
help 用于询问机器人会什么或怎么用。只有 inventory、low_stock、out_of_stock、movements 可以包含 keyword。
非库存相关问题返回 unknown。keyword 只保留商品名称或编码，不得生成 SQL。`

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
		MaxTokens: 80,
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
		Intent  string `json:"intent"`
		Keyword string `json:"keyword"`
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
	if len([]rune(result.Keyword)) > larkKeywordMaxRunes {
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
	case "inventory_value_ranking":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionValueRanking, Name: "自然语言·库存金额排行"}, nil
	case "movements":
		return larkCommand{Action: larkActionMovements, Name: "自然语言·查流水", Keyword: result.Keyword}, nil
	case "today_changes":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionTodayChanges, Name: "自然语言·今日变动"}, nil
	case "today_sales_ranking":
		if result.Keyword != "" {
			return larkCommand{}, errDeepSeekIntent
		}
		return larkCommand{Action: larkActionTodaySalesRanking, Name: "自然语言·今日销售排行"}, nil
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
