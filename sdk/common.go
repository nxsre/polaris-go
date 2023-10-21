package sdk

import (
	"context"
	jsoniter "github.com/json-iterator/go"
	"github.com/nxsre/polaris-go"
	"net/url"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func PolarisUrl(uri string) string {
	fullUrl, _ := url.JoinPath(baseUrl, uri)
	return fullUrl
}

type SDK struct {
	polarisClient *polaris.Polaris
	ctx           context.Context
}

func NewSDK(ctx context.Context, client *polaris.Polaris) *SDK {
	return &SDK{
		polarisClient: client,
		ctx:           ctx,
	}
}
