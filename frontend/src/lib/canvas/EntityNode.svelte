<script lang="ts">
  import { Handle, Position, type NodeProps } from "@xyflow/svelte";
  import {
    Users,
    Image,
    Clapperboard,
    ListVideo,
    Film,
    Clock,
    ArrowUpRight,
  } from "@lucide/svelte";
  import { mediaUrl, isImage } from "$lib/api";
  import { KIND_LABELS, kindColor } from "./transform";
  let { data, selected }: NodeProps = $props();
  const icons: Record<string, typeof Users> = {
    character: Users,
    asset: Image,
    scene: Clapperboard,
    shot: ListVideo,
    render_job: Film,
    output: Film,
  };
  const kind = $derived(String(data.kind ?? ""));
  const Icon = $derived(icons[kind] ?? Film);
  const label = $derived(String(data.label || "Untitled"));
  const isDerived = $derived(kind === "render_job" || kind === "output");
  const preview = $derived(
    kind === "asset" && isImage(data.file_path as string)
      ? mediaUrl(data.file_path as string)
      : kind === "output" && data.thumbnail_path
        ? mediaUrl(data.thumbnail_path as string)
        : null,
  );
  const excerpt = $derived(
    String(
      (kind === "character"
        ? data.content || data.description
        : kind === "scene"
          ? data.summary || data.prompt
          : data.prompt || data.description) || "",
    ),
  );
  const hint = $derived(
    kind === "character"
      ? "Add appearance & wardrobe"
      : kind === "scene"
        ? "Define the setting and action"
        : kind === "shot"
          ? "Add camera and motion direction"
          : kind === "asset"
            ? "Add a reference or design brief"
            : kind === "output"
              ? "Private video · download or edit"
              : "Generation progress",
  );
</script>

<div
  class="entity-card"
  class:entity-selected={selected}
  class:entity-derived={isDerived}
  class:entity-character={kind === "character"}
>
  <Handle
    id="l"
    type="source"
    position={Position.Left}
    class="entity-handle"
    title="Drag to connect"
  />
  <div class="entity-kind">
    <Icon size={12} /><span>{KIND_LABELS[kind] || kind}</span
    >{#if Number(data.duration) > 0}<span class="entity-duration"
        ><Clock size={10} />{data.duration}s</span
      >{/if}<span class="entity-dot" style={`background:${kindColor(kind)}`}
    ></span>
  </div>
  {#if preview}<img
      src={preview}
      alt=""
      class="entity-preview"
      loading="lazy"
    />{/if}
  <div class="entity-body">
    {#if kind === "character"}<div class="entity-avatar" aria-hidden="true">
        {label.slice(0, 2).toUpperCase()}
      </div>{/if}
    <div class="entity-copy">
      <strong title={label}>{label}</strong>
      <p class:entity-hint={!excerpt}>{excerpt || hint}</p>
    </div>
  </div>
  <div class="entity-footer">
    {#if kind === "render_job"}<span class:failed={data.status === "failed"}
        >{String(data.status || "queued").replaceAll("_", " ")}</span
      >{:else if kind === "asset"}<span>Private reference</span
      >{:else if kind === "output"}<span>Finished output</span>{:else}<span
        >{excerpt ? "Editable design" : "Add details"}</span
      >{/if}<ArrowUpRight size={12} />
  </div>
  <Handle
    id="r"
    type="source"
    position={Position.Right}
    class="entity-handle"
    title="Drag to connect"
  />
</div>

<style>
  .entity-card {
    width: 240px;
    background: var(--card);
    color: var(--card-foreground);
    border: 1px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 3px 10px #00000012;
    transition:
      border-color 0.15s,
      box-shadow 0.15s;
  }
  .entity-selected {
    border-color: var(--primary);
    box-shadow: 0 3px 16px #00000020;
  }
  .entity-derived {
    background: var(--muted);
  }
  .entity-kind {
    height: 32px;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 12px;
    font-size: 10px;
    color: var(--muted-foreground);
    border-bottom: 1px solid var(--border);
  }
  .entity-dot {
    height: 5px;
    width: 5px;
    border-radius: 50%;
    margin-left: auto;
  }
  .entity-duration {
    display: flex;
    gap: 3px;
    align-items: center;
    margin-left: auto;
    font-variant-numeric: tabular-nums;
  }
  .entity-duration + .entity-dot {
    margin-left: 3px;
  }
  .entity-body {
    display: flex;
    gap: 11px;
    padding: 13px 12px;
    min-height: 77px;
  }
  .entity-copy {
    min-width: 0;
    flex: 1;
  }
  .entity-copy strong {
    font-size: 12px;
    font-weight: 600;
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
  }
  .entity-copy p {
    font-size: 10px;
    line-height: 1.65;
    color: var(--muted-foreground);
    margin: 5px 0 0;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
  }
  .entity-copy .entity-hint {
    font-size: 10px;
  }
  .entity-avatar {
    width: 43px;
    height: 52px;
    display: grid;
    place-items: center;
    border-radius: 8px;
    background: var(--muted);
    color: var(--foreground);
    font-size: 17px;
    font-weight: 500;
    flex-shrink: 0;
  }
  .entity-preview {
    display: block;
    width: 100%;
    height: 124px;
    object-fit: cover;
  }
  .entity-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 7px 12px;
    border-top: 1px solid var(--border);
    font-size: 9px;
    color: var(--muted-foreground);
    min-height: 28px;
  }
  .failed {
    color: var(--destructive);
  }
  :global(.svelte-flow__handle.entity-handle) {
    width: 14px;
    height: 14px;
    border-radius: 9999px;
    background: var(--muted-foreground);
    border: 3px solid var(--background);
    z-index: 20;
    pointer-events: all;
  }
  :global(.svelte-flow__handle.entity-handle:hover) {
    background: var(--primary);
  }
</style>
