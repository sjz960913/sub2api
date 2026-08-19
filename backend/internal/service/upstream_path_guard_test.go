package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSanitizedUpstreamPathSuffixRejectsNonConformingSegments(t *testing.T) {
	// 到达业务代码的 URL.Path 已是百分号解码后的结果，因此用例按解码后的形态书写。
	rejected := []string{
		"/..",
		"/../..",
		"/../../x/y",
		"/./compact",
		"/compact/..",
		`/..\..\x`,
		`/compact\..`,
		"/?a=b",
		"/compact?a=b",
		"/compact#frag",
		"/compact%2f..",
		"/100%",
		"//double",
		"/compact//detail",
		"/compact/",
		"/ compact",
		"/compact\x00",
		"/compact\nX-Injected: 1",
		"/模型",
		"compact",
		"/a:b",
		"/a;b",
		"/a,b",
		"/a=b",
		"/a&b",
		// 允许清单是闭集：`\w` + `-` + `.` 以外的字符一律拒绝，
		// 不依赖任何"已知坏字符"清单。
		"/a~b",
		"/a@b",
		"/a+b",
		"/a|b",
		"/a*b",
		"/a$b",
		"/a(b)",
		"/a'b",
		"/a\"b",
		"/a<b",
		"/a\tb",
		"/a b",
		"/a∕b", // DIVISION SLASH
		"/a／b", // FULLWIDTH SOLIDUS
		// 只由点组成的片段一律拒绝（各实现对其解释不一致）。
		"/...",
		"/....",
		"/compact/...",
	}
	for _, suffix := range rejected {
		t.Run("reject_"+suffix, func(t *testing.T) {
			got, ok := sanitizedUpstreamPathSuffix(suffix)
			require.False(t, ok, "suffix %q must be rejected", suffix)
			require.Empty(t, got)
		})
	}

	accepted := map[string]string{
		"":                          "",
		"/compact":                  "/compact",
		"/compact/detail":           "/compact/detail",
		"/resp_68f0a1b2c3d4/cancel": "/resp_68f0a1b2c3d4/cancel",
		"/gemini-2.5-pro_v1.2":      "/gemini-2.5-pro_v1.2",
		"/a.b.c":                    "/a.b.c",
	}
	for suffix, want := range accepted {
		t.Run("accept_"+suffix, func(t *testing.T) {
			got, ok := sanitizedUpstreamPathSuffix(suffix)
			require.True(t, ok, "suffix %q must be accepted", suffix)
			require.Equal(t, want, got)
		})
	}
}

func TestSanitizedUpstreamPathSuffixEnforcesBounds(t *testing.T) {
	longSegment := "/"
	for i := 0; i < maxUpstreamPathSegmentLen+1; i++ {
		longSegment += "a"
	}
	_, ok := sanitizedUpstreamPathSuffix(longSegment)
	require.False(t, ok, "over-long segment must be rejected")

	deep := ""
	for i := 0; i <= maxUpstreamPathSegments; i++ {
		deep += "/a"
	}
	_, ok = sanitizedUpstreamPathSuffix(deep)
	require.False(t, ok, "over-deep suffix must be rejected")
}

// TestOpenAIResponsesRequestPathSuffixRejectsNonConformingSubpaths 锁定不变式：
// /responses 只允许裸路径和精确 /compact；格式安全但未列入协议白名单的路径
// 同样 fail closed。
func TestOpenAIResponsesRequestPathSuffixRejectsNonConformingSubpaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	nonConformingPaths := []string{
		"/v1/responses/../../x/y",
		"/v1/responses/..%2f..%2fx/y",
		"/v1/responses/%2e%2e/%2e%2e/x",
		"/responses/%2e%2e%2fx",
		"/backend-api/codex/responses/../../../x",
		`/v1/responses/..\..\x`,
		"/v1/responses/%3fa=b",
		"/v1/responses/x%23frag",
		"/v1/responses//double",
		"/v1/responses/",
		"/responses/compact/",
		"/v1/responses/compact/detail",
		"/v1/responses/resp_123/cancel",
		"/v1/responses/future_metadata_channel",
	}
	for _, path := range nonConformingPaths {
		t.Run(path, func(t *testing.T) {
			c := newResponsesSuffixTestContext(t, path)

			require.False(t, IsForwardableOpenAIResponsesRequestPath(c),
				"path %q must be rejected at the gateway edge", path)
			require.Empty(t, openAIResponsesRequestPathSuffix(c),
				"path %q must never contribute an upstream path suffix", path)
			require.Equal(t, chatgptCodexURL,
				appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, openAIResponsesRequestPathSuffix(c)))
			require.False(t, isOpenAIResponsesCompactPath(c))
		})
	}

	// 合法子路径必须保持原样转发。
	for path, want := range map[string]string{
		"/v1/responses":                        "",
		"/v1/responses/compact":                "/compact",
		"/backend-api/codex/responses/compact": "/compact",
	} {
		t.Run("forwardable_"+path, func(t *testing.T) {
			c := newResponsesSuffixTestContext(t, path)
			require.True(t, IsForwardableOpenAIResponsesRequestPath(c))
			require.Equal(t, want, openAIResponsesRequestPathSuffix(c))
		})
	}
}

func TestIsOpenAIResponsesCompactPathUsesLegacyEndpointShape(t *testing.T) {
	legacyPaths := []string{
		"/v1/responses/compact",
	}
	for _, path := range legacyPaths {
		t.Run("legacy_"+path, func(t *testing.T) {
			c := newResponsesSuffixTestContext(t, path)
			require.True(t, IsOpenAIResponsesCompactPath(c))
		})
	}

	nonLegacyPaths := []string{
		"/v1/responses",
		"/openai/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
		"/responses/compact/",
		"/v1/responses/compact/detail",
		"/v1/responses/resp_123/cancel",
	}
	for _, path := range nonLegacyPaths {
		t.Run("non_legacy_"+path, func(t *testing.T) {
			c := newResponsesSuffixTestContext(t, path)
			require.False(t, IsOpenAIResponsesCompactPath(c))
		})
	}
}

func TestAppendOpenAIResponsesRequestPathSuffixRefusesUnsafeSuffix(t *testing.T) {
	// 调用方漏了校验时，拼接函数本身也不得把不合规片段带进上游 URL。
	require.Equal(t, chatgptCodexURL, appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/../../x"))
	require.Equal(t, chatgptCodexURL, appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/?a=b"))
	require.Equal(t, chatgptCodexURL, appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/compact/detail"))
	require.Equal(t, chatgptCodexURL+"/compact", appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/compact"))
}

func newResponsesSuffixTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}
