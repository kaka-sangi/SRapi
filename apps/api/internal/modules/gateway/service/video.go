package service

import (
	"fmt"
	"strings"
	"time"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	defaultVideoSeconds        = 4
	maxVideoSeconds            = 15
	maxVideoReferenceImageURLs = 7
)

func (s *Service) NormalizeVideoCreate(req apiopenapi.VideoCreateRequest, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("video model is empty")
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("video prompt is empty")
	}
	seconds := defaultVideoSeconds
	if req.Duration != nil {
		seconds = *req.Duration
	}
	if req.Seconds != nil {
		seconds = *req.Seconds
	}
	if seconds < 1 || seconds > maxVideoSeconds {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("video seconds must be between 1 and %d", maxVideoSeconds)
	}
	inputReference := firstNonEmptyString(
		videoReferenceURL(req.InputReference),
		videoReferenceURL(req.Image),
		stringPtrValue(req.ImageUrl),
	)
	references := videoReferenceURLs(req.ReferenceImages)
	if req.ReferenceImageUrls != nil {
		for _, url := range *req.ReferenceImageUrls {
			if value := strings.TrimSpace(url); value != "" {
				references = append(references, value)
			}
		}
	}
	if inputReference != "" && len(references) > 0 {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("video input_reference cannot be combined with reference_images")
	}
	if len(references) > maxVideoReferenceImageURLs {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("video reference_images cannot exceed %d", maxVideoReferenceImageURLs)
	}
	extra := cloneMap(req.AdditionalProperties)
	delete(extra, "duration")
	delete(extra, "reference_image_urls")
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, model, "", false, prompt, nil, imageContentBlocks(prompt), "", nil)
	canonical.VideoPrompt = prompt
	canonical.VideoSeconds = seconds
	canonical.VideoSize = stringPtrValue(req.Size)
	canonical.VideoAspectRatio = stringPtrValue(req.AspectRatio)
	canonical.VideoResolution = stringPtrValue(req.Resolution)
	canonical.VideoInputReference = inputReference
	canonical.VideoReferenceImages = append([]string(nil), references...)
	canonical.VideoUser = stringPtrValue(req.User)
	canonical.VideoExtra = extra
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyVideos, Version: "v1"})
	return canonical, nil
}

func (s *Service) BuildCanonicalVideoResponse(req gatewaycontract.CanonicalRequest, video gatewaycontract.VideoResponse, usage gatewaycontract.Usage) gatewaycontract.VideoResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	if video.Status == "" {
		video.Status = gatewaycontract.VideoStatusQueued
	}
	if shouldEstimateUsage(usage) {
		usage = videoEstimatedUsage(req)
	}
	video.RequestID = strings.TrimSpace(req.RequestID)
	video.Model = model
	video.CanonicalModel = canonicalModel
	video.Usage = usage
	video.CompatibilityWarnings = uniqueStrings(req.CompatibilityWarnings)
	if video.CreatedAt == nil {
		now := time.Now().Unix()
		video.CreatedAt = &now
	}
	return video
}

func (s *Service) RenderVideo(resp gatewaycontract.VideoResponse) apiopenapi.VideoObject {
	out := apiopenapi.VideoObject{
		Id:                   resp.ID,
		Model:                resp.Model,
		Object:               apiopenapi.Video,
		Status:               apiopenapi.VideoObjectStatus(resp.Status),
		AdditionalProperties: cloneMap(resp.Metadata),
	}
	out.Progress = cloneInt(resp.Progress)
	if value := strings.TrimSpace(resp.Prompt); value != "" {
		out.Prompt = &value
	}
	out.Seconds = cloneInt(resp.Seconds)
	if value := strings.TrimSpace(resp.Size); value != "" {
		out.Size = &value
	}
	out.CreatedAt = cloneInt64(resp.CreatedAt)
	out.CompletedAt = cloneInt64(resp.CompletedAt)
	out.ExpiresAt = cloneInt64(resp.ExpiresAt)
	if resp.Error != nil {
		out.Error = &apiopenapi.VideoError{
			Code:                 optionalStringPtr(resp.Error.Code),
			Message:              optionalStringPtr(resp.Error.Message),
			AdditionalProperties: cloneMap(resp.Error.Metadata),
		}
	}
	return out
}

func videoEstimatedUsage(req gatewaycontract.CanonicalRequest) gatewaycontract.Usage {
	return gatewaycontract.Usage{
		InputTokens: estimateTokens(req.VideoPrompt),
		Estimated:   true,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func videoReferenceURLs(values *[]apiopenapi.VideoReference) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(*values))
	for _, value := range *values {
		if url := videoReferenceURL(&value); url != "" {
			out = append(out, url)
		}
	}
	return out
}

func videoReferenceURL(value *apiopenapi.VideoReference) string {
	if value == nil {
		return ""
	}
	if text, err := value.AsVideoReference0(); err == nil {
		return strings.TrimSpace(string(text))
	}
	object, err := value.AsVideoReference1()
	if err != nil {
		return ""
	}
	if object.Url != nil && strings.TrimSpace(*object.Url) != "" {
		return strings.TrimSpace(*object.Url)
	}
	if object.ImageUrl != nil {
		if url, err := object.ImageUrl.AsVideoReference1ImageUrl0(); err == nil {
			return strings.TrimSpace(string(url))
		}
		if nested, err := object.ImageUrl.AsVideoReference1ImageUrl1(); err == nil && nested.Url != nil {
			return strings.TrimSpace(*nested.Url)
		}
	}
	if object.FileId != nil {
		return strings.TrimSpace(*object.FileId)
	}
	return ""
}

func optionalStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
