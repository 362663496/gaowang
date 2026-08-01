package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
)

func Test_DeepSeek_maps_common_inventory_language_to_validated_query_plans(t *testing.T) {
	tests := []struct {
		input   string
		result  string
		action  string
		keyword string
		plan    *larkAnalyticsPlan
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
		{input: "总库存有多少件", result: `{"intent":"analytics","metric":"inventory_quantity","group_by":"none","time_range":"current","sort":null,"limit":null}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricInventoryQuantity, GroupBy: larkGroupNone, TimeRange: larkTimeCurrent, Limit: 1}},
		{input: "整体库存金额是多少", result: `{"intent":"analytics","metric":"inventory_value"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricInventoryValue, GroupBy: larkGroupNone, TimeRange: larkTimeCurrent, Limit: 1}},
		{input: "现在有多少个商品", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "有多少商品没库存", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "看下整体库存情况", result: `{"intent":"inventory_summary"}`, action: larkActionSummary},
		{input: "库存金额最高的商品有哪些", result: `{"intent":"analytics","metric":"inventory_value","group_by":"product","time_range":"current","sort":"desc","limit":5}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricInventoryValue, GroupBy: larkGroupProduct, TimeRange: larkTimeCurrent, Sort: "desc", Limit: 5}},
		{input: "按库存价值排个前五", result: `{"intent":"analytics","metric":"inventory_value","group_by":"product"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricInventoryValue, GroupBy: larkGroupProduct, TimeRange: larkTimeCurrent, Sort: "desc", Limit: 5}},
		{input: "库存最少的两个商品", result: `{"intent":"analytics","metric":"inventory_quantity","group_by":"product","sort":"asc","limit":2}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricInventoryQuantity, GroupBy: larkGroupProduct, TimeRange: larkTimeCurrent, Sort: "asc", Limit: 2}},
		{input: "最近谁动过库存", result: `{"intent":"movements"}`, action: larkActionMovements},
		{input: "绿茶最近的出入库记录", result: `{"intent":"movements","keyword":"绿茶"}`, action: larkActionMovements, keyword: "绿茶"},
		{input: "TEA-1 最近有没有操作", result: `{"intent":"movements","keyword":"TEA-1"}`, action: larkActionMovements, keyword: "TEA-1"},
		{input: "给我最近五条流水", result: `{"intent":"movements"}`, action: larkActionMovements},
		{input: "今天入库多少", result: `{"intent":"analytics","metric":"movement_quantity","movement_type":"inbound","time_range":"today"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupNone, MovementType: larkMovementInbound, TimeRange: larkTimeToday, Limit: 1}},
		{input: "今天出库多少", result: `{"intent":"analytics","metric":"movement_quantity","movement_type":"sales_outbound","time_range":"today"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupNone, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Limit: 1}},
		{input: "今天调整了几次", result: `{"intent":"analytics","metric":"movement_count","movement_type":"adjustment","time_range":"today"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementCount, GroupBy: larkGroupNone, MovementType: larkMovementAdjustment, TimeRange: larkTimeToday, Limit: 1}},
		{input: "今天库存总变动", result: `{"intent":"today_changes"}`, action: larkActionTodayChanges},
		{input: "今天什么卖得最好", result: `{"intent":"analytics","metric":"movement_quantity","group_by":"product","movement_type":"sales_outbound","time_range":"today","sort":"desc","limit":1}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupProduct, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Sort: "desc", Limit: 1}},
		{input: "今日销售数量排行", result: `{"intent":"analytics","metric":"movement_quantity","group_by":"product","movement_type":"sales_outbound","time_range":"today"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupProduct, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Sort: "desc", Limit: 5}},
		{input: "哪个店铺出货最多", result: `{"intent":"analytics","metric":"movement_quantity","group_by":"shop","movement_type":"sales_outbound","time_range":"all","sort":"desc","limit":1}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupShop, MovementType: larkMovementOutbound, TimeRange: larkTimeAll, Sort: "desc", Limit: 1}},
		{input: "近7天各店铺入库排行", result: `{"intent":"analytics","metric":"movement_quantity","group_by":"shop","movement_type":"inbound","time_range":"last_n_days","days":7}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupShop, MovementType: larkMovementInbound, TimeRange: larkTimeLastNDays, Days: 7, Sort: "desc", Limit: 5}},
		{input: "昨天一共出了多少货", result: `{"intent":"analytics","metric":"movement_quantity","movement_type":"sales_outbound","time_range":"yesterday"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupNone, MovementType: larkMovementOutbound, TimeRange: larkTimeYesterday, Limit: 1}},
		{input: "本月谁操作次数最多", result: `{"intent":"analytics","metric":"movement_count","group_by":"operator","movement_type":"all","time_range":"current_month","limit":1}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementCount, GroupBy: larkGroupOperator, MovementType: larkMovementAll, TimeRange: larkTimeCurrentMonth, Sort: "desc", Limit: 1}},
		{input: "所有操作人按流水次数排行", result: `{"intent":"analytics","metric":"movement_count","group_by":"operator","movement_type":"all","time_range":"all"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementCount, GroupBy: larkGroupOperator, MovementType: larkMovementAll, TimeRange: larkTimeAll, Sort: "desc", Limit: 5}},
		{input: "张三今天出库多少", result: `{"intent":"analytics","metric":"movement_quantity","movement_type":"sales_outbound","time_range":"today","operator_keyword":"张三"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupNone, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Limit: 1, OperatorKeyword: "张三"}},
		{input: "A店本月出库金额", result: `{"intent":"analytics","metric":"movement_value","movement_type":"sales_outbound","time_range":"current_month","shop_keyword":"A店"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementValue, GroupBy: larkGroupNone, MovementType: larkMovementOutbound, TimeRange: larkTimeCurrentMonth, Limit: 1, ShopKeyword: "A店"}},
		{input: "绿茶累计入库多少", result: `{"intent":"analytics","metric":"movement_quantity","movement_type":"inbound","time_range":"all","product_keyword":"绿茶"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupNone, MovementType: larkMovementInbound, TimeRange: larkTimeAll, Limit: 1, ProductKeyword: "绿茶"}},
		{input: "本月各店铺调整净数量", result: `{"intent":"analytics","metric":"movement_quantity","group_by":"shop","movement_type":"adjustment","time_range":"current_month"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementQuantity, GroupBy: larkGroupShop, MovementType: larkMovementAdjustment, TimeRange: larkTimeCurrentMonth, Sort: "desc", Limit: 5}},
		{input: "近30天商品出库金额排行", result: `{"intent":"analytics","metric":"movement_value","group_by":"product","movement_type":"sales_outbound","time_range":"last_n_days","days":30}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementValue, GroupBy: larkGroupProduct, MovementType: larkMovementOutbound, TimeRange: larkTimeLastNDays, Days: 30, Sort: "desc", Limit: 5}},
		{input: "今天每个店铺销售了几笔", result: `{"intent":"analytics","metric":"movement_count","group_by":"shop","movement_type":"sales_outbound","time_range":"today"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Metric: larkMetricMovementCount, GroupBy: larkGroupShop, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Sort: "desc", Limit: 5}},
		{input: "今天按商品排序并列出店铺各占多少", result: `{"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"ranking","group_by":["product","shop"],"movement_type":"sales_outbound","time_range":"today","sort":"desc","limit":10,"presentation":"matrix"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationRanking, GroupBy: larkGroupProduct, GroupBy2: larkGroupShop, MovementType: larkMovementOutbound, TimeRange: larkTimeToday, Sort: "desc", Limit: 10, Presentation: larkPresentationMatrix}},
		{input: "本月各店铺销量占比", result: `{"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"share","group_by":["shop"],"movement_type":"sales_outbound","time_range":"current_month","limit":10,"presentation":"share"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationShare, GroupBy: larkGroupShop, MovementType: larkMovementOutbound, TimeRange: larkTimeCurrentMonth, Sort: "desc", Limit: 10, Presentation: larkPresentationShare}},
		{input: "今年每月出库趋势", result: `{"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"trend","group_by":[],"movement_type":"sales_outbound","time_range":"current_year","time_bucket":"month","limit":10,"presentation":"trend"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationTrend, GroupBy: larkGroupNone, MovementType: larkMovementOutbound, TimeRange: larkTimeCurrentYear, TimeBucket: larkBucketMonth, Sort: "desc", Limit: 10, Presentation: larkPresentationTrend}},
		{input: "7月销售和上期相比如何", result: `{"intent":"analytics","domain":"movement","metric":"movement_quantity","operation":"comparison","group_by":[],"movement_type":"sales_outbound","time_range":"date_range","date_from":"2026-07-01","date_to":"2026-07-31","compare_to":"previous_period","limit":10,"presentation":"comparison"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Domain: larkDomainMovement, Metric: larkMetricMovementQuantity, Operation: larkOperationComparison, GroupBy: larkGroupNone, MovementType: larkMovementOutbound, TimeRange: larkTimeDateRange, DateFrom: "2026-07-01", DateTo: "2026-07-31", CompareTo: larkComparePreviousPeriod, Sort: "desc", Limit: 10, Presentation: larkPresentationComparison}},
		{input: "列出晴朗店铺资料", result: `{"intent":"analytics","domain":"shop","operation":"details","group_by":[],"time_range":"current","limit":10,"presentation":"detail","shop_keyword":"晴朗"}`, action: larkActionAnalytics,
			plan: &larkAnalyticsPlan{Domain: larkDomainShop, Operation: larkOperationDetails, GroupBy: larkGroupNone, TimeRange: larkTimeCurrent, Sort: "desc", Limit: 10, Presentation: larkPresentationDetail, ShopKeyword: "晴朗"}},
		{input: "你会干什么", result: `{"intent":"help"}`, action: larkActionHelp},
		{input: "明天天气怎么样", result: `{"intent":"unknown"}`, action: larkActionUnknown},
	}
	if len(tests) < 40 {
		t.Fatalf("natural-language cases = %d, want at least 40", len(tests))
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
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.Model != "deepseek-v4-flash" || payload.Thinking.Type != "disabled" || payload.ResponseFormat.Type != "json_object" || payload.MaxTokens < 200 ||
			len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" {
			t.Errorf("request payload = %+v", payload)
		}
		if strings.Contains(payload.Messages[0].Content, "售价") ||
			!strings.Contains(payload.Messages[0].Content, "out_of_stock") ||
			!strings.Contains(payload.Messages[0].Content, "analytics") ||
			!strings.Contains(payload.Messages[0].Content, "group_by") ||
			!strings.Contains(payload.Messages[0].Content, "哪个店铺出货最多") ||
			strings.Contains(payload.Messages[0].Content, "inventory_value_ranking") ||
			strings.Contains(payload.Messages[0].Content, "today_sales_ranking") {
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
			if command.Action != test.action || command.Keyword != test.keyword || !reflect.DeepEqual(command.Analytics, test.plan) {
				t.Fatalf("command = %+v, want action=%s keyword=%q plan=%+v", command, test.action, test.keyword, test.plan)
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
		{name: "legacy narrow ranking intent", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"today_sales_ranking"}`)},
		{name: "keyword on analytics", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","keyword":"茶","metric":"inventory_value"}`)},
		{name: "analytics missing metric", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","group_by":"shop"}`)},
		{name: "invalid analytics metric", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"sale_price"}`)},
		{name: "inventory grouped by shop", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"inventory_quantity","group_by":"shop"}`)},
		{name: "inventory with movement type", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"inventory_value","movement_type":"inbound"}`)},
		{name: "movement with current time", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","time_range":"current"}`)},
		{name: "last n days missing days", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","time_range":"last_n_days"}`)},
		{name: "last n days too large", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","time_range":"last_n_days","days":366}`)},
		{name: "days on today", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","time_range":"today","days":7}`)},
		{name: "invalid group", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","group_by":"warehouse"}`)},
		{name: "invalid sort", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","group_by":"shop","sort":"random"}`)},
		{name: "limit too large", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","group_by":"shop","limit":11}`)},
		{name: "filter too long", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"analytics","metric":"movement_count","product_keyword":"` + longKeyword + `"}`)},
		{name: "plan fields on help", status: http.StatusOK, body: deepSeekCompletion(`{"intent":"help","metric":"movement_count"}`)},
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
