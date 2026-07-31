package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

func Test_DeepSeek_maps_common_inventory_language_to_whitelisted_commands(t *testing.T) {
	tests := []struct {
		input   string
		result  string
		action  string
		keyword string
	}{
		{input: "绿茶还有多少库存？", result: `{"intent":"inventory","keyword":"绿茶"}`, action: larkActionInventory, keyword: "绿茶"},
		{input: "帮我看一下 TEA-1 的价格", result: `{"intent":"inventory","keyword":"TEA-1"}`, action: larkActionInventory, keyword: "TEA-1"},
		{input: "BR-1214G 有没有货", result: `{"intent":"inventory","keyword":"BR-1214G"}`, action: larkActionInventory, keyword: "BR-1214G"},
		{input: "绿茶现在状态正常吗", result: `{"intent":"inventory","keyword":"绿茶"}`, action: larkActionInventory, keyword: "绿茶"},
		{input: "绿茶的库存值多少钱", result: `{"intent":"inventory","keyword":"绿茶"}`, action: larkActionInventory, keyword: "绿茶"},
		{input: "查商品 BR-1214G", result: `{"intent":"inventory","keyword":"BR-1214G"}`, action: larkActionInventory, keyword: "BR-1214G"},
		{input: "哪些商品快没了？", result: `{"intent":"low_stock"}`, action: larkActionLowStock},
		{input: "哪些需要补货", result: `{"intent":"low_stock"}`, action: larkActionLowStock},
		{input: "列一下低于库存线的商品", result: `{"intent":"low_stock"}`, action: larkActionLowStock},
		{input: "当前低库存有多少种", result: `{"intent":"low_stock"}`, action: larkActionLowStock},
		{input: "哪些商品已经没货了", result: `{"intent":"out_of_stock"}`, action: larkActionOutOfStock},
		{input: "把零库存商品列出来", result: `{"intent":"out_of_stock"}`, action: larkActionOutOfStock},
		{input: "茶类里哪些缺货", result: `{"intent":"out_of_stock","keyword":"茶"}`, action: larkActionOutOfStock, keyword: "茶"},
		{input: "总库存有多少件", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "整体库存金额是多少", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "现在有多少个商品", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "有多少商品没库存", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "看下整体库存情况", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "库存金额最高的商品有哪些", result: `{"intent":"inventory_value_ranking"}`, action: larkActionValueRanking},
		{input: "按库存价值排个前五", result: `{"intent":"inventory_value_ranking"}`, action: larkActionValueRanking},
		{input: "哪些货压的钱最多", result: `{"intent":"inventory_value_ranking"}`, action: larkActionValueRanking},
		{input: "最近谁动过库存", result: `{"intent":"movements"}`, action: larkActionMovements},
		{input: "绿茶最近的出入库记录", result: `{"intent":"movements","keyword":"绿茶"}`, action: larkActionMovements, keyword: "绿茶"},
		{input: "TEA-1 最近有没有操作", result: `{"intent":"movements","keyword":"TEA-1"}`, action: larkActionMovements, keyword: "TEA-1"},
		{input: "给我最近五条流水", result: `{"intent":"movements"}`, action: larkActionMovements},
		{input: "今天入库多少", result: `{"intent":"today_changes"}`, action: larkActionTodayChanges},
		{input: "今天出库多少", result: `{"intent":"today_changes"}`, action: larkActionTodayChanges},
		{input: "今天调整了几次", result: `{"intent":"today_changes"}`, action: larkActionTodayChanges},
		{input: "今天库存总变动", result: `{"intent":"today_changes"}`, action: larkActionTodayChanges},
		{input: "今天什么卖得最好", result: `{"intent":"today_sales_ranking"}`, action: larkActionTodaySalesRanking},
		{input: "今日销售数量排行", result: `{"intent":"today_sales_ranking"}`, action: larkActionTodaySalesRanking},
		{input: "今天卖出去最多的商品", result: `{"intent":"today_sales_ranking"}`, action: larkActionTodaySalesRanking},
		{input: "你会干什么", result: `{"intent":"help"}`, action: larkActionHelp},
		{input: "明天天气怎么样", result: `{"intent":"unknown"}`, action: larkActionUnknown},
	}
	if len(tests) < 30 {
		t.Fatalf("natural-language cases = %d, want at least 30", len(tests))
	}
	results := make(map[string]string, len(tests))
	for _, test := range tests {
		results[test.input] = test.result
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var payload struct {
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
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.Model != "deepseek-v4-flash" || payload.Thinking.Type != "disabled" || payload.ResponseFormat.Type != "json_object" ||
			len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" {
			t.Errorf("request payload = %+v", payload)
		}
		if strings.Contains(payload.Messages[0].Content, "售价") ||
			!strings.Contains(payload.Messages[0].Content, "out_of_stock") ||
			!strings.Contains(payload.Messages[0].Content, "inventory_value_ranking") ||
			!strings.Contains(payload.Messages[0].Content, "today_sales_ranking") {
			t.Errorf("system prompt = %q", payload.Messages[0].Content)
		}
		result, ok := results[payload.Messages[1].Content]
		if !ok {
			t.Errorf("unexpected user content %q", payload.Messages[1].Content)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]string{"content": result},
			}},
		})
	}))
	defer server.Close()
	parser := &deepSeekIntentParser{apiKey: "test-key", model: "deepseek-v4-flash", endpoint: server.URL, client: server.Client()}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			command, err := parser.Parse(context.Background(), test.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if command.Action != test.action || command.Keyword != test.keyword {
				t.Fatalf("command = %+v, want action=%s keyword=%q", command, test.action, test.keyword)
			}
		})
	}
}

func Test_Lark_all_text_messages_use_deepseek_without_fixed_command_priority(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"intent\":\"inventory_summary\"}"}}]}`))
	}))
	defer server.Close()
	bot := larkBot{intentParser: &deepSeekIntentParser{apiKey: "key", model: "deepseek-v4-flash", endpoint: server.URL, client: server.Client()}}
	mention := larktypes.Mention{Key: "@bot", IsBot: true}

	command, err := bot.resolveCommand(context.Background(), larktypes.NormalizedMessage{
		RawContentType: "text", Content: "@bot 查库存 BR-1214G", Mentions: []larktypes.Mention{mention},
	})
	if err != nil || command.Action != larkActionSummary || requests != 1 {
		t.Fatalf("first AI command = %+v err=%v requests=%d", command, err, requests)
	}
	command, err = bot.resolveCommand(context.Background(), larktypes.NormalizedMessage{
		RawContentType: "text", Content: "@bot 帮助", Mentions: []larktypes.Mention{mention},
	})
	if err != nil || command.Action != larkActionSummary || requests != 2 {
		t.Fatalf("second AI command = %+v err=%v requests=%d", command, err, requests)
	}
}

func Test_Lark_missing_deepseek_key_degrades_all_text_but_keeps_empty_boundaries_local(t *testing.T) {
	bot := larkBot{}
	mention := larktypes.Mention{Key: "@bot", IsBot: true}
	_, err := bot.resolveCommand(context.Background(), larktypes.NormalizedMessage{
		RawContentType: "text", Content: "@bot 低库存", Mentions: []larktypes.Mention{mention},
	})
	if !errors.Is(err, errDeepSeekIntent) {
		t.Fatalf("text command error = %v", err)
	}
	for _, message := range []larktypes.NormalizedMessage{
		{RawContentType: "text", Content: "@bot", Mentions: []larktypes.Mention{mention}},
		{RawContentType: "image", Content: "[image]", Mentions: []larktypes.Mention{mention}},
	} {
		command, boundaryErr := bot.resolveCommand(context.Background(), message)
		if boundaryErr != nil || command.Action != larkActionHelp {
			t.Fatalf("boundary command = %+v err=%v", command, boundaryErr)
		}
	}
	card, cardErr := larkAIUnavailableCard()
	if cardErr != nil || strings.Contains(card, "固定命令") || !strings.Contains(card, "没有执行查询或库存操作") {
		t.Fatalf("fallback card = %s err=%v", card, cardErr)
	}
}

func Test_DeepSeek_rejects_untrusted_or_failed_responses_without_leaking_secret(t *testing.T) {
	longKeyword := strings.Repeat("茶", larkKeywordMaxRunes+1)
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "non 2xx", status: http.StatusTooManyRequests, body: `do-not-leak-this-body`},
		{name: "invalid completion json", status: http.StatusOK, body: `{`},
		{name: "empty choices", status: http.StatusOK, body: `{"choices":[]}`},
		{name: "truncated", status: http.StatusOK, body: `{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`},
		{name: "empty content", status: http.StatusOK, body: `{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`},
		{name: "invalid result json", status: http.StatusOK, body: `{"choices":[{"finish_reason":"stop","message":{"content":"{"}}]}`},
		{name: "unsupported intent", status: http.StatusOK, body: `{"choices":[{"finish_reason":"stop","message":{"content":"{\"intent\":\"write_stock\"}"}}]}`},
		{name: "missing inventory keyword", status: http.StatusOK, body: `{"choices":[{"finish_reason":"stop","message":{"content":"{\"intent\":\"inventory\"}"}}]}`},
		{name: "keyword too long", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"inventory","keyword":"` + longKeyword + `"}`)},
		{name: "unknown field", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"help","sql":"DROP"}`)},
		{name: "keyword on summary", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"inventory_summary","keyword":"茶"}`)},
		{name: "keyword on ranking", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"today_sales_ranking","keyword":"茶"}`)},
		{name: "multiple objects", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"help"}{"intent":"unknown"}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.WriteHeader(test.status)
				_, _ = response.Write([]byte(test.body))
			}))
			defer server.Close()
			parser := &deepSeekIntentParser{apiKey: "do-not-leak-this-key", model: "deepseek-v4-flash", endpoint: server.URL, client: server.Client()}

			_, err := parser.Parse(context.Background(), "查库存")
			if !errors.Is(err, errDeepSeekIntent) || strings.Contains(err.Error(), "do-not-leak") {
				t.Fatalf("Parse() error = %v", err)
			}
		})
	}

	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			<-release
		}))
		defer server.Close()
		parser := &deepSeekIntentParser{
			apiKey: "secret", model: "deepseek-v4-flash", endpoint: server.URL,
			client: &http.Client{Timeout: 10 * time.Millisecond},
		}
		_, err := parser.Parse(context.Background(), "库存怎么样")
		close(release)
		if !errors.Is(err, errDeepSeekIntent) {
			t.Fatalf("Parse() error = %v", err)
		}
	})

	if parser := newDeepSeekIntentParser("", "deepseek-v4-flash"); parser != nil {
		t.Fatalf("empty API key parser = %#v", parser)
	}
}

func deepSeekCompletion(content string) string {
	payload, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{
			"finish_reason": "stop",
			"message":       map[string]string{"content": content},
		}},
	})
	return string(payload)
}
