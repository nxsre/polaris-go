package configfiles

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"github.com/nxsre/polaris-go"
	"github.com/nxsre/polaris-go/sdk"
	"log"
)

type ConfigFile struct {
	// 自动发布配置文件必要的字段
	ReleaseName        string              `json:"release_name,omitempty"`
	ReleaseDescription string              `json:"release_description,omitempty"`
	Comment            string              `json:"comment,omitempty"`
	Format             string              `json:"format,omitempty"`
	FileName           string              `json:"file_name"`
	Namespace          string              `json:"namespace"`
	Group              string              `json:"group"`
	Content            string              `json:"content"`
	Tags               []sdk.ConfigFileTag `json:"tags"`
}

func CreateAndPub(config *ConfigFile) (*ConfigFileResult, error) {
	body, err := jsoniter.Marshal(config)
	if err != nil {
		return nil, err
	}
	resp, err := polaris.DefaultClient.Resty().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(sdk.PolarisUrl("/config/v1/configfiles/createandpub"))
	if err != nil {
		return nil, err
	}
	result := ConfigFileResult{}
	if err := jsoniter.Unmarshal(resp.Body(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func Delete(ns, group, fileName string) {
	fileUri := fmt.Sprintf("/config/v1/configfiles")
	resp, err := polaris.DefaultClient.Resty().R().
		SetHeader("Content-Type", "application/json").
		SetQueryParams(map[string]string{
			"namespace": ns,
			"group":     group,
			"name":      fileName,
		}).
		Delete(sdk.PolarisUrl(fileUri))
	log.Println(fileUri, resp.String(), err)
}

type ConfigFileResult struct {
	Code                     int    `json:"code"`
	Info                     string `json:"info"`
	ConfigFileGroup          any    `json:"configFileGroup"`
	ConfigFile               any    `json:"configFile"`
	ConfigFileRelease        any    `json:"configFileRelease"`
	ConfigFileReleaseHistory any    `json:"configFileReleaseHistory"`
	ConfigFileTemplate       any    `json:"configFileTemplate"`
}
