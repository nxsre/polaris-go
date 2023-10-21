package sdk

type ConfigFileMetadataListRequest struct {
	ConfigFileGroup ConfigFileGroup `json:"config_file_group"`
}
type ConfigFileGroup struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type ConfigFileMetadataListResult struct {
	Code            int          `json:"code"`
	Info            string       `json:"info"`
	Revision        string       `json:"revision"`
	Namespace       string       `json:"namespace"`
	Group           string       `json:"group"`
	ConfigFileInfos []ConfigFile `json:"config_file_infos"`
}

// GetConfigFileMetadata 获取分组下的文件列表
func (s *SDK) GetConfigFileMetadataList(ns, group string) (*ConfigFileMetadataListResult, error) {
	resp, err := s.polarisClient.Resty().R().SetBody(&ConfigFileMetadataListRequest{
		ConfigFileGroup: ConfigFileGroup{
			Namespace: ns,
			Name:      group,
		}}).Post(PolarisUrl("/config/v1/GetConfigFileMetadataList"))
	if err != nil {
		return nil, err
	}

	result := &ConfigFileMetadataListResult{}
	err = json.Unmarshal(resp.Body(), result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
