# Browser studio: character to finished video

This guide covers the single-user browser app in this repository. A hosted server must be rebuilt/redeployed after a GitHub update; refreshing the old server does not install new code.

1. Activate a project. Add characters and upload reference photos. Confirm their thumbnails and reference counts.
2. On Create video, choose the scene count and cast explicitly. Start with one scene. The preflight shows configured OpenRouter models and supported clip durations; unsupported reference/aspect combinations fail before the guided pipeline starts paid work.
3. Enter the story, language, style, aspect and direction. The effective brief and character/reference snapshot are kept with the generated scenes. The Canvas and Scenes story forms also let you select the cast.
4. Review scenes and shots before further generation. Regenerating scene shots replaces the previous shot snapshot atomically; it does not append duplicate shots. A concurrent edit causes the late result to be rejected instead of overwriting approved work.
Changing a scene’s cast invalidates its approved keyframes and refreshes the character/reference snapshot.
5. Generate keyframes with a reference-capable image model. A first-frame-capable video model receives the approved scene keyframe. A reference image is conditioning material, not a guarantee of exact identity or deterministic camera cuts.
6. Generate scene or shot clips. One scene render is one provider-generated clip, not a deterministic edit of individually rendered shots. Unsupported planned durations fail rather than silently truncating. My videos and Activity show progress and outputs.
7. For captions, open an output's editor: transcribe or write timed text, edit it, then Save. Save completes only after a new captioned file is produced. A failed burn keeps the previous export. The Canvas caption card burns existing saved captions; transcription model/language settings belong in the editor.
8. In My videos, select approved outputs in playback order, choose the combined aspect ratio, optionally require captioned versions for every clip, and choose Assemble selected clips. Local FFmpeg normalizes dimensions/audio, adds silent audio where needed, then concatenates without another video encode. The combined output opens in the editor and can be downloaded.

## Failure and recovery

- A browser request keeps its request key after a lost network response. Replaying it does not automatically repeat the same paid action.
- If a video submission might have been accepted but its result is unknown, the job requires reconciliation. Check OpenRouter and resume the verified existing job from My videos; this does not submit another video. Do not start another generation merely because the response was lost.
- Settings shows interrupted generation responses when no current server request owns them. Review Activity, saved outputs and provider activity before explicitly unlocking; record the evidence. Unlocking starts nothing, cancels nothing and refunds nothing. A subsequent generation can incur a new charge.
- Server restart marks interrupted non-render background work as requiring reconciliation and retains saved work. It does not repeat AI stages blindly. Known provider video IDs resume polling; uncertain submission intents remain fenced.
- Re-render is an explicit new paid generation using saved inputs and requires confirmation. It is unavailable while the original job is active or uncertain. Repeated retries of an old job return its active replacement.

Local verification uses mocked provider responses plus real FFmpeg. Actual model fidelity, current model availability and the deployed server/browser still require a small, budgeted live check. The SaaS repositories and their cloud deployment are separate from this local workflow.
