import React, { useState } from "react";
import { EditorCore } from "@/core";
import { useEditor } from "@/editor/use-editor";
import { storageService } from "@/services/storage/service";
import { processMediaAssets } from "@/media/processing";
export type SourceRecord = {
  id: string;
  name: string;
  type: string;
  size: number;
};
let manifest: SourceRecord[] = [];
export function restoreSourceManifest(value: unknown) {
  manifest = Array.isArray(value)
    ? value.filter(
        (x): x is SourceRecord =>
          !!x && typeof x.id === "string" && typeof x.name === "string",
      )
    : [];
}
export function sourceManifest() {
  const assets = EditorCore.getInstance().media.getAssets();
  const merged = new Map(manifest.map((x) => [x.id, x]));
  for (const asset of assets)
    merged.set(asset.id, {
      id: asset.id,
      name: asset.name,
      type: asset.type,
      size: asset.file.size,
    });
  manifest = [...merged.values()];
  return manifest;
}
export function missingSources() {
  const editor = EditorCore.getInstance();
  const available = new Set(editor.media.getAssets().map((x) => x.id));
  const referenced = new Set<string>();
  function visit(value: unknown) {
    if (!value || typeof value !== "object") return;
    if ("mediaId" in value && typeof value.mediaId === "string")
      referenced.add(value.mediaId);
    for (const child of Object.values(value)) visit(child);
  }
  visit(editor.scenes.getScenes());
  return [...referenced]
    .filter((id) => !available.has(id))
    .map(
      (id) =>
        manifest.find((x) => x.id === id) || {
          id,
          name: "Original source file",
          type: "",
          size: 0,
        },
    );
}
export function SourceSafety() {
  const missing = useEditor(() => missingSources().map((x) => x.id));
  const editor = useEditor();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  if (!missing.length) return null;
  return (
    <section className="vf-source-warning" role="alert">
      <strong>
        Reconnect {missing.length} missing source{" "}
        {missing.length === 1 ? "file" : "files"} to export.
      </strong>
      <span>
        Timelines sync to your account. Video, audio and image files stay in the
        browser where you imported them.
      </span>
      {missingSources().map((source) => (
        <label key={source.id}>
          {source.name}
          <input
            type="file"
            disabled={!!busy}
            aria-label={`Reconnect ${source.name}`}
            onChange={async (event) => {
              const file = event.target.files?.[0];
              if (!file) return;
              setBusy(source.id);
              setError("");
              try {
                if (source.size && file.size !== source.size)
                  throw Error(
                    "Choose the original file: its size differs from this source.",
                  );
                const [asset] = await processMediaAssets({ files: [file] });
                if (!asset) throw Error("This file could not be decoded.");
                if (source.type && asset.type !== source.type)
                  throw Error("Choose the same type of source file.");
                await storageService.saveMediaAsset({
                  projectId: editor.project.getActive().metadata.id,
                  mediaAsset: { ...asset, id: source.id },
                });
                await editor.media.loadProjectMedia({
                  projectId: editor.project.getActive().metadata.id,
                });
              } catch (e) {
                setError((e as Error).message);
              } finally {
                setBusy("");
              }
            }}
          />
        </label>
      ))}
      {error && <p>{error}</p>}
    </section>
  );
}
