<script lang="ts">
  import { page } from "$app/state";
  import { selectStudioProject } from "./navigation";
  import { onMount } from "svelte";
  import EntityCanvas from "$lib/canvas/EntityCanvas.svelte";
  import { PaneGroup, Pane, Handle } from "$lib/components/ui/resizable";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Textarea } from "$lib/components/ui/textarea";
  import { Label } from "$lib/components/ui/label";
  import {
    MessageSquare,
    Send,
    RefreshCw,
    Plus,
    LoaderCircle,
  } from "@lucide/svelte";
  import { request } from "./api";
  import { setCanvasProject } from "./legacy";
  import GenerationJobs from "./GenerationJobs.svelte";
  import PromptWorkbench from "./prompts/PromptWorkbench.svelte";
  import { promptBytes } from "./prompts/render.mjs";
  import { Users, Image, ChevronRight } from "@lucide/svelte";
  import type { Project, Quote } from "./generated";
  let projects = $state<Project[]>([]);
  let project = $state("");
  let projectName = $state("");
  let canvas = $state<EntityCanvas>();
  let error = $state("");
  let busy = $state(false);
  let loaded = $state(false);
  let mobile = $state(false);
  let showAssistant = $state(true);
  let idea = $state("");
  let clips = $state(2);
  let seconds = $state(5);
  let aspect = $state("16:9");
  let quote = $state<Quote | null>(null);
  let key = "";
  let quoteIdea = "";
  let jobs = $state<any[]>([]);
  let message = $state("");
  let storageKey = "";
  let cast = $state<any[]>([]),
    projectScenes = $state<any[]>([]),
    assets = $state<any[]>([]),
    includeContext = $state(true),
    generationLanguage = $state<"en" | "zh">("en");
  const context = $derived(
    [
      ...cast.map(
        (c) =>
          `Character: ${c.title}\n${c.description}\n${c.content}\n${c.prompt}`,
      ),
      ...projectScenes.map(
        (s) => `Scene: ${s.title}\n${s.description}\n${s.prompt}`,
      ),
    ].join("\n\n"),
  );
  const finalIdea = $derived(
    idea +
      (includeContext && context
        ? "\n\nConfirmed project design context (text only; use as continuity notes, do not render design sheets):\n" +
          context
        : "") +
      "\n\nWrite the screenplay and scene titles in " +
      (generationLanguage === "zh"
        ? "Simplified Chinese (Mandarin)."
        : "English.") +
      " Preserve quoted dialogue in its original language.",
  );
  async function refresh() {
    if (!project) return;
    const id = project;
    const [j, c, s, a] = await Promise.all([
      request(`/projects/${id}/jobs`),
      request(`/projects/${id}/characters`),
      request(`/projects/${id}/scenes`),
      request(`/projects/${id}/assets`),
    ]);
    if (id !== project) return;
    jobs = j;
    cast = c;
    projectScenes = s;
    assets = a;
    await canvas?.refresh();
  }
  function select(id: string) {
    project = id;
    setCanvasProject(id);
    quote = null;
    jobs = [];
    cast = [];
    projectScenes = [];
    assets = [];
    error = "";
    if (storageKey) localStorage.setItem(storageKey, id);
    void refresh().catch((e) => (error = e.message));
  }
  async function createProject() {
    busy = true;
    error = "";
    try {
      const p = await request("/projects", {
        name: projectName,
        description: "",
      });
      projects = [p, ...projects];
      projectName = "";
      select(p.id);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  async function prepare() {
    busy = true;
    error = "";
    quote = null;
    quoteIdea = finalIdea;
    try {
      quote = await request(`/projects/${project}/pipelines/quotes`, {
        idea: finalIdea,
        clips: Number(clips),
        clip_duration: Number(seconds),
        aspect_ratio: aspect,
      });
      key = crypto.randomUUID();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  async function generate() {
    if (!quote) return;
    if (quoteIdea !== finalIdea) {
      quote = null;
      error = "Your prompt changed. Request a new credit quote.";
      return;
    }
    busy = true;
    error = "";
    try {
      await request("/generation/jobs", { quote_id: quote.id }, "POST", {
        "Idempotency-Key": key,
      });
      quote = null;
      message =
        "Your video is processing. The screenplay, scenes and shots will appear on the canvas.";
      await refresh();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    let mounted = true;
    const media = window.matchMedia("(max-width: 767px)");
    const resize = () => (mobile = media.matches);
    resize();
    media.addEventListener("change", resize);
    void Promise.all([request("/me"), request("/projects")])
      .then(([user, p]) => {
        if (!mounted) return;
        projects = p;
        storageKey = `videoflow.cloud.project.${user.id}`;
        const saved = localStorage.getItem(storageKey);
        select(
          selectStudioProject(
            projects,
            page.url.searchParams.get("project"),
            saved,
          ),
        );
      })
      .catch((e) => (error = e.message))
      .finally(() => (loaded = true));
    return () => {
      mounted = false;
      media.removeEventListener("change", resize);
      setCanvasProject("");
    };
  });
</script>

<div class="flex h-full min-h-0 flex-col bg-background text-foreground">
  <header
    class="flex min-h-12 flex-wrap items-center gap-3 border-b border-border px-4 py-2"
  >
    <label for="canvas-project" class="text-sm font-medium">Project</label>
    <select
      id="canvas-project"
      class="h-8 max-w-64 rounded-md border border-input bg-background px-2 text-sm"
      value={project}
      onchange={(e) => select(e.currentTarget.value)}
      disabled={!projects.length}
      ><option value="" disabled>Select a project</option
      >{#each projects as p}<option value={p.id}>{p.name}</option
        >{/each}</select
    >
    <Button
      size="sm"
      variant="ghost"
      onclick={() => refresh().catch((e) => (error = e.message))}
      disabled={!project}
      ><RefreshCw class="size-4" /><span class="sr-only">Refresh canvas</span
      ></Button
    >
    <Button
      size="sm"
      variant="ghost"
      onclick={() => (showAssistant = !showAssistant)}
      aria-pressed={!showAssistant}
      >{showAssistant ? "Focus canvas" : "Show story panel"}</Button
    >
    <a
      href={`/studio/editor?project=${encodeURIComponent(project)}`}
      class="ml-auto text-sm text-muted-foreground underline underline-offset-4"
      >Open editor</a
    ><a
      href="/studio/dashboard"
      class=" text-sm text-muted-foreground underline underline-offset-4"
      >Studio overview</a
    >
  </header>
  {#if error}<div
      role="alert"
      class="border-b border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive"
    >
      {error}
    </div>{/if}
  {#if !loaded}<div class="grid flex-1 place-items-center">
      <LoaderCircle class="size-5 animate-spin" />
    </div>
  {:else if !project}<div class="grid flex-1 place-items-center p-6">
      <form
        class="grid w-full max-w-sm gap-4"
        onsubmit={(e) => {
          e.preventDefault();
          void createProject();
        }}
      >
        <h1 class="text-xl font-semibold">Make your first video</h1>
        <p class="text-sm text-muted-foreground">
          Create a project to start arranging scenes, characters and shots on
          your canvas.
        </p>
        <Label for="canvas-project-name">Project name</Label><Input
          id="canvas-project-name"
          bind:value={projectName}
          maxlength={120}
          required
        /><Button type="submit" disabled={busy}
          ><Plus class="size-4" />Create project</Button
        >
      </form>
    </div>
  {:else}
    <PaneGroup
      direction={mobile ? "vertical" : "horizontal"}
      class="min-h-0 flex-1"
    >
      <Pane defaultSize={mobile ? 55 : 68} minSize={35}
        ><div class="relative h-full min-h-80">
          {#key project}<EntityCanvas
              bind:this={canvas}
              onselect={() => {}}
              onupdated={refresh}
            />{/key}
        </div></Pane
      >
      {#if showAssistant}<Handle withHandle />
        <Pane defaultSize={mobile ? 45 : 32} minSize={20}
          ><aside
            class="flex h-full flex-col border-l border-border"
            aria-label="Video assistant"
          >
            <header
              class="flex items-center justify-between gap-2 border-b border-border px-4 py-3 text-sm font-medium"
            >
              <span class="flex items-center gap-2"
                ><MessageSquare class="size-4" />Story direction</span
              ><a
                href={`/characters?project=${encodeURIComponent(project)}`}
                class="text-xs text-muted-foreground">Manage cast</a
              >
            </header>
            <div class="flex-1 overflow-y-auto p-4">
              <div class="canvas-designs">
                <div>
                  <h2>Project designs</h2>
                  <span>{cast.length} characters · {assets.length} assets</span>
                </div>
                {#each cast.slice(0, 4) as character}<button
                    onclick={() => canvas?.focusNode(character.id)}
                    ><span>{character.title.slice(0, 2).toUpperCase()}</span
                    ><strong>{character.title}</strong><ChevronRight
                      size={13}
                    /></button
                  >{/each}{#if assets.length}<details class="design-asset-list">
                    <summary>Project assets · {assets.length}</summary
                    >{#each assets.slice(0, 6) as asset}<button
                        type="button"
                        onclick={() => canvas?.focusNode(asset.id)}
                        ><Image size={14} /><span>{asset.title}</span
                        ><ChevronRight size={12} /></button
                      >{/each}
                  </details>{/if}{#if !cast.length}<p>
                    Add characters to keep identity and wardrobe consistent.
                  </p>
                  <a href={`/characters?project=${encodeURIComponent(project)}`}
                    >Create a character</a
                  >{/if}
              </div>
              <div class="mb-5">
                {#key project}<PromptWorkbench
                    projectId={project}
                    category="story"
                    onapply={(text, language) => {
                      idea = text;
                      generationLanguage = language;
                      quote = null;
                    }}
                  />{/key}
              </div>
              <p class="mb-5 text-sm text-muted-foreground">
                Describe your story. Review the credit quote before VideoFlow
                writes the screenplay, generates the clips and assembles your
                video.
              </p>
              {#if message}<p
                  role="status"
                  class="mb-4 rounded-md bg-muted p-3 text-sm"
                >
                  {message}
                </p>{/if}
              <form
                class="grid gap-4"
                onsubmit={(e) => {
                  e.preventDefault();
                  void prepare();
                }}
              >
                <div class="grid gap-2">
                  <Label for="canvas-idea">Your story</Label><Textarea
                    id="canvas-idea"
                    rows={5}
                    bind:value={idea}
                    required
                    maxlength={8000}
                    placeholder="A small idea, a character, a world…"
                    oninput={() => (quote = null)}
                  />
                </div>
                <div class="grid gap-2">
                  <Label for="generation-language"
                    >Script language / 剧本语言</Label
                  ><select
                    id="generation-language"
                    bind:value={generationLanguage}
                    onchange={() => (quote = null)}
                    class="h-9 rounded-md border border-input bg-background px-2 text-sm"
                    ><option value="en">English</option><option value="zh"
                      >简体中文 · Mandarin</option
                    ></select
                  >
                </div>
                <label
                  class="flex items-start gap-2 text-xs text-muted-foreground"
                  ><input
                    type="checkbox"
                    bind:checked={includeContext}
                    onchange={() => (quote = null)}
                    class="mt-0.5 accent-primary"
                  />Use saved project characters and scene directions (text
                  only)</label
                >
                <details class="canvas-prompt-review">
                  <summary
                    >Review submitted prompt <span
                      >{promptBytes(finalIdea)}/8,000</span
                    ></summary
                  >
                  <pre>{finalIdea}</pre>
                </details>
                <div class="grid grid-cols-2 gap-3">
                  <div class="grid gap-2">
                    <Label for="canvas-clips">Scenes</Label><Input
                      id="canvas-clips"
                      type="number"
                      min={1}
                      max={4}
                      bind:value={clips}
                      oninput={() => (quote = null)}
                    />
                  </div>
                  <div class="grid gap-2">
                    <Label for="canvas-seconds">Seconds per scene</Label><Input
                      id="canvas-seconds"
                      type="number"
                      min={5}
                      max={10}
                      bind:value={seconds}
                      oninput={() => (quote = null)}
                    />
                  </div>
                </div>
                <div class="grid gap-2">
                  <Label for="canvas-aspect">Aspect ratio</Label><select
                    id="canvas-aspect"
                    bind:value={aspect}
                    onchange={() => (quote = null)}
                    class="h-9 rounded-md border border-input bg-background px-2 text-sm"
                    ><option>16:9</option><option>9:16</option><option
                      >1:1</option
                    ></select
                  >
                </div>
                <p class="text-xs text-muted-foreground">
                  Up to 30 seconds total. Models are managed by VideoFlow.
                  Generation credits are separate from Premium.
                </p>
                <Button
                  type="submit"
                  disabled={busy ||
                    clips * seconds > 30 ||
                    promptBytes(finalIdea) > 8000}
                  ><Send class="size-4" />{busy
                    ? "Preparing…"
                    : "Get credit quote"}</Button
                >
              </form>
              {#if quote}<section
                  class="mt-4 grid gap-3 rounded-lg border border-border p-4"
                  aria-label="Confirm generation"
                >
                  <strong class="text-sm"
                    >{quote.credits.toLocaleString()} credits</strong
                  >
                  <p class="text-sm text-muted-foreground">
                    Credits are reserved when you confirm and charged after the
                    finished video succeeds.
                  </p>
                  <Button disabled={busy} onclick={generate}
                    >Confirm and generate</Button
                  ><Button variant="ghost" onclick={() => (quote = null)}
                    >Cancel quote</Button
                  >
                </section>{/if}
              <GenerationJobs {jobs} onrefresh={refresh} />
            </div>
          </aside></Pane
        >{/if}
    </PaneGroup>
  {/if}
</div>

<style>
  .design-asset-list {
    margin-top: 10px;
  }
  .design-asset-list summary {
    font-size: 11px;
    color: var(--muted-foreground);
    cursor: pointer;
    padding: 7px 0;
  }
  .design-asset-list button {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 0;
    width: 100%;
    text-align: left;
    font-size: 11px;
  }
  .design-asset-list button > span {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .canvas-designs {
    margin-bottom: 20px;
    border-bottom: 1px solid var(--border);
    padding-bottom: 18px;
  }
  .canvas-designs > div {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 12px;
  }
  .canvas-designs h2 {
    font-size: 12px;
    font-weight: 600;
  }
  .canvas-designs > div > span {
    font-size: 10px;
    color: var(--muted-foreground);
  }
  .canvas-designs > button {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 7px 0;
    text-align: left;
  }
  .canvas-designs > button > span {
    display: grid;
    place-items: center;
    width: 30px;
    height: 34px;
    background: var(--muted);
    border-radius: 6px;
    font-size: 11px;
  }
  .canvas-designs > button strong {
    font-size: 12px;
    font-weight: 500;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .canvas-designs > button:hover strong {
    color: var(--primary);
  }
  .canvas-designs p {
    font-size: 11px;
    line-height: 1.8;
    color: var(--muted-foreground);
  }
  .canvas-designs > a {
    display: inline-block;
    font-size: 11px;
    margin-top: 8px;
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .canvas-prompt-review summary {
    font-size: 11px;
    cursor: pointer;
    color: var(--muted-foreground);
  }
  .canvas-prompt-review summary span {
    float: right;
    font-variant-numeric: tabular-nums;
  }
  .canvas-prompt-review pre {
    font-family: inherit;
    font-size: 11px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 220px;
    overflow: auto;
    padding: 12px 0;
    line-height: 1.8;
  }
</style>
