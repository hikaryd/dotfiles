package beatapi

import (
	"errors"
	"slices"
	"strings"
)

var chatModels = []string{"glm-5.3-free", "glm-5.3-flash-free", "deepseek-v4.1-flash-free", "deepseek-v4-flash-0731-free", "gemini-3.8-flash-free", "gpt-6.1-sol-free"}
var errModel = errors.New("Unsupported model for this endpoint; only listed free models are allowed")

func resolveModel(model string, systemone bool) (string, error) {
	model = strings.TrimPrefix(model, "beatapi/")
	if (systemone && model == jevModel) || (!systemone && slices.Contains(chatModels, model)) {
		return model, nil
	}
	return "", errModel
}

func listedModels() []map[string]string {
	models := make([]map[string]string, 0, 2*(len(chatModels)+1))
	for _, id := range append(slices.Clone(chatModels), jevModel) {
		for _, name := range []string{id, "beatapi/" + id} {
			models = append(models, map[string]string{"id": name, "object": "model", "owned_by": "beatapi"})
		}
	}
	return models
}
