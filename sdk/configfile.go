package sdk

import (
	"encoding/base64"
	"errors"
	"github.com/nxsre/polaris-go/crypto"
	"github.com/nxsre/polaris-go/log"
	"github.com/polarismesh/specification/source/go/api/v1/model"

	"strconv"
)

const (
	// ConfigFileTagKeyUseEncrypted 配置加密开关标识，value 为 boolean
	ConfigFileTagKeyUseEncrypted = "internal-encrypted"
	// ConfigFileTagKeyDataKey 加密密钥 tag key
	ConfigFileTagKeyDataKey = "internal-datakey"
	// ConfigFileTagKeyEncryptAlgo 加密算法 tag key
	ConfigFileTagKeyEncryptAlgo = "internal-encryptalgo"

	baseUrl = "http://polaris.com"
)

type ConfigFile struct {
	// 基础字段
	Namespace string          `json:"namespace"`
	Group     string          `json:"group"`
	FileName  string          `json:"fileName"`
	Content   string          `json:"content,omitempty"`
	Tags      []ConfigFileTag `json:"tags,omitempty"`

	// 查询返回
	Version     string `json:"version,omitempty"`
	Md5         string `json:"md5,omitempty"`
	Encrypted   bool   `json:"encrypted,omitempty"`
	PublicKey   string `json:"publicKey,omitempty"`
	Name        string `json:"name,omitempty"`
	ReleaseTime any    `json:"release_time,omitempty"`
}

type ConfigFileTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GetNamespace 获取配置文件命名空间
func (c *ConfigFile) GetNamespace() string {
	return c.Namespace
}

// GetFileGroup 获取配置文件组
func (c *ConfigFile) GetFileGroup() string {
	return c.Group
}

// GetFileName 获取配置文件名
func (c *ConfigFile) GetFileName() string {
	return c.FileName
}

// GetSourceContent 获取配置文件内容
func (c *ConfigFile) GetSourceContent() string {
	return c.Content
}

// GetVersion 获取配置文件版本号
func (c *ConfigFile) GetVersion() uint64 {
	version, err := strconv.ParseUint(c.Version, 10, 64)
	if err != nil {
		log.Errorln(err)
		return 0
	}
	return version
}

// GetMd5 获取配置文件MD5值
func (c *ConfigFile) GetMd5() string {
	return c.Md5
}

// GetEncrypted 获取配置文件是否为加密文件
func (c *ConfigFile) GetEncrypted() bool {
	return c.Encrypted
}

// GetPublicKey 获取配置文件公钥
func (c *ConfigFile) GetPublicKey() string {
	return c.PublicKey
}

// GetDataKey 获取配置文件数据加密密钥
func (c *ConfigFile) GetDataKey() string {
	for _, tag := range c.Tags {
		if tag.Key == ConfigFileTagKeyDataKey {
			return tag.Value
		}
	}
	return ""
}

// GetContent 获取配置文件内容
func (c *ConfigFile) GetContent() (string, error) {
	if c.GetEncrypted() {
		switch c.GetEncryptAlgo() {
		case "AES":
			aes := crypto.AesCryptor{}
			key, err := base64.StdEncoding.DecodeString(c.GetDataKey())
			if err != nil {
				return "", err
			}
			return aes.Decrypt(c.Content, key)
		default:
			return "", errors.New("not support algo")
		}
	}
	return c.Content, nil
}

// GetEncryptAlgo 获取配置文件数据加密算法
func (c *ConfigFile) GetEncryptAlgo() string {
	for _, tag := range c.Tags {
		if tag.Key == ConfigFileTagKeyEncryptAlgo {
			return tag.Value
		}
	}
	return ""
}

func (s *SDK) GetConfigFile(ns, group, filename string) (*ConfigFileResponse, error) {
	resp, err := s.polarisClient.Resty().R().SetQueryParams(map[string]string{
		"namespace": ns,
		"group":     group,
		"fileName":  filename,
	}).Get(PolarisUrl("/config/v1/GetConfigFile"))
	if err != nil {
		log.Errorln(resp, err)
		return nil, err
	}
	result := &ConfigFileResponse{}
	err = json.Unmarshal(resp.Body(), result)
	if err != nil {
		log.Errorln(resp, err)
		return nil, err
	}

	return result, nil
}

// ConfigFileResponse 配置文件响应体
type ConfigFileResponse struct {
	Code       model.Code
	Info       string
	ConfigFile *ConfigFile `json:"configFile"`
}

// GetCode 获取配置文件响应体code
func (c *ConfigFileResponse) GetCode() model.Code {
	return c.Code
}

// GetMessage 获取配置文件响应体信息
func (c *ConfigFileResponse) GetMessage() string {
	return c.Info
}

// GetConfigFile 获取配置文件响应体内容
func (c *ConfigFileResponse) GetConfigFile() *ConfigFile {
	return c.ConfigFile
}
