package httpx

import "context"

// reqInfo is shared, mutable per-request state written by inner layers and read
// by the outer logging layer after the handler returns.
type reqInfo struct {
	route    string
	clientIP string
	subject  string
}

type infoKey struct{}

func withInfo(ctx context.Context, i *reqInfo) context.Context {
	return context.WithValue(ctx, infoKey{}, i)
}

func info(ctx context.Context) *reqInfo {
	if i, ok := ctx.Value(infoKey{}).(*reqInfo); ok {
		return i
	}
	return &reqInfo{}
}

// ClientIP returns the resolved client IP of the request in ctx.
func ClientIP(ctx context.Context) string { return info(ctx).clientIP }

// SetSubject records the authenticated subject for access logging.
func SetSubject(ctx context.Context, subject string) { info(ctx).subject = subject }
