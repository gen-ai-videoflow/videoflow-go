package api

import (
	"context"
	"encoding/json"
)

func (s *Server) generationPrerequisites(ctx context.Context, brief generationBrief) []map[string]string {
	missing := []map[string]string{}
	add := func(route, reason string) {
		missing = append(missing, map[string]string{"route": route, "setting": "/settings", "reason": reason})
	}
	client, err := s.routes().Client("openrouter")
	if err != nil {
		add("authentication", err.Error())
		return missing
	}
	for _, agent := range []string{"script_agent", "shot_agent", "prompt_agent"} {
		if _, _, err = s.routes().Agent(ctx, agent); err != nil {
			add(agent, err.Error())
		}
	}
	image, err := client.Model(ctx, s.routes().MediaModel("image"), "image")
	if err != nil {
		add("image", err.Error())
	} else if len(brief.ReferenceAssetIDs) > 0 {
		var caps map[string]json.RawMessage
		json.Unmarshal(image.SupportedParameters, &caps)
		if caps["input_references"] == nil {
			add("image", "Selected image model does not support character references; choose a reference-capable image model before generating")
		}
	}
	imagePayload := map[string]any{"model": s.routes().MediaModel("image"), "prompt": "preflight", "aspect_ratio": brief.AspectRatio}
	if len(brief.ReferenceAssetIDs) > 0 {
		refs := []map[string]any{}
		for range brief.ReferenceAssetIDs {
			refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,iVBORw0KGgo="}})
		}
		imagePayload["input_references"] = refs
	}
	if err := client.ValidateImage(ctx, imagePayload); err != nil {
		add("image", err.Error())
	}
	video, err := client.Model(ctx, s.routes().MediaModel("video"), "video")
	if err != nil {
		add("video", err.Error())
		return missing
	}
	supported := false
	for _, a := range video.AspectRatios {
		supported = supported || a == brief.AspectRatio
	}
	if !supported {
		add("video", "Selected video model does not support aspect ratio "+brief.AspectRatio)
	}
	if brief.SceneCount > 0 && brief.TargetDuration > 0 {
		max := 0
		for _, duration := range video.Durations {
			if duration > max {
				max = duration
			}
		}
		target := (brief.TargetDuration + brief.SceneCount - 1) / brief.SceneCount
		if target > max {
			add("duration", "Planned average scene length exceeds this model's supported clip duration; shorten the target, add scenes, or select a suitable model")
		}
	}
	if len(brief.ReferenceAssetIDs) > 0 {
		conditioned := false
		for _, f := range video.FrameImages {
			conditioned = conditioned || f == "first_frame"
		}
		if !conditioned {
			add("video", "Selected video model cannot use the character-conditioned keyframe; select a first-frame model")
		}
	}
	return missing
}
