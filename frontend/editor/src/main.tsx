import {
  SourceSafety,
  restoreSourceManifest,
  sourceManifest,
} from "./source-safety";
import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "sonner";
import { EditorCore } from "@/core";
import { EditorProvider } from "@/components/providers/editor-provider";
import { MediaView } from "@/components/editor/panels/assets/views/assets";
import { TextView } from "@/text/components/assets-view";
import { EffectsView } from "@/effects/components/assets-view";
import { Captions } from "@/subtitles/components/assets-view";
import { SettingsView } from "@/components/editor/panels/assets/views/settings";
import { PropertiesPanel } from "@/components/editor/panels/properties";
import { Timeline } from "@/timeline/components";
import { PreviewPanel } from "@/preview/components";
import { ExportButton } from "@/components/editor/export-button";
import { useEditor } from "@/editor/use-editor";
import { storageService } from "@/services/storage/service";
import { processMediaAssets } from "@/media/processing";
import {
  FolderOpen,
  Type,
  Captions as CaptionIcon,
  Sparkles,
  Settings,
  ArrowLeft,
} from "lucide-react";
import "@/globals.css";
import "./videoflow.css";
const query = new URLSearchParams(location.search);
const project = query.get("project") || "";
const owner = query.get("owner") || "";
let localId = "";
let revision = 0;
let cloudSaveBlocked = false;
let saveQueue = Promise.resolve();
let flushRequest = "";
async function api(path: string, body?: unknown) {
  const r = await fetch("/api/v1" + path, {
    credentials: "same-origin",
    ...(body
      ? {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }
      : {}),
  });
  if (!r.ok) {
    if (r.status === 409) cloudSaveBlocked = true;
    throw new Error(
      r.status === 409
        ? "Another tab saved this timeline. Reload before saving again."
        : `Cloud request failed (${r.status}).`,
    );
  }
  return r.json();
}
async function backup() {
  await EditorCore.getInstance().save.flush();
  const data = await (storageService as any).projectsAdapter.get(localId);
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(data, null, 2)], { type: "application/json" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = "videoflow-timeline.json";
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function cloudSnapshot(value: any) {
  const { thumbnail, ...metadata } = value.metadata;
  return { ...value, metadata, sourceManifest: sourceManifest() };
}
function parent(type: string, payload: any = {}) {
  window.parent.postMessage({ type, ...payload }, location.origin);
}
function App() {
  const [ready, setReady] = useState(false),
    [error, setError] = useState(""),
    [name, setName] = useState(""),
    [tab, setTab] = useState("Media"),
    [status, setStatus] = useState("Loading project…");
  useEffect(() => {
    let active = true;
    (async () => {
      if (!project || !owner)
        throw Error("Open the editor from your VideoFlow project.");
      const [me, projects, saved] = await Promise.all([
        api("/me"),
        api("/projects"),
        api(`/projects/${encodeURIComponent(project)}/editor`),
      ]);
      if ((me.user || me).id !== owner)
        throw Error("Your account changed. Reopen the editor.");
      const p = projects.find((p: any) => p.id === project);
      if (!p) throw Error("Project not found.");
      setName(p.name);
      revision = saved.revision;
      restoreSourceManifest(saved.document?.sourceManifest);
      const cacheKey = `videoflow.editor.${owner}.${project}`;
      const editor = EditorCore.getInstance();
      const cached = localStorage.getItem(cacheKey);
      const cachedProject = cached
        ? await storageService.loadProject({ id: cached })
        : null;
      const pendingKey = cacheKey + ".pending";
      const pending = localStorage.getItem(pendingKey);
      if (pending && !cachedProject)
        throw Error(
          "Unsynced timeline cache is unavailable. This browser will not overwrite the cloud timeline.",
        );
      if (pending && Number(pending) !== saved.revision)
        cloudSaveBlocked = true;
      if (saved.document && !pending) {
        localId = saved.document.metadata.id;
        if (cached && cached !== localId)
          throw Error(
            "Local editor identity differs. Reopen in a fresh browser profile or restore your project backup.",
          );
        const raw = saved.document;
        await (storageService as any).projectsAdapter.set({
          key: localId,
          value: raw,
        });
      } else if (cachedProject) {
        localId = cached!;
      } else {
        localId = await editor.project.createNewProject({ name: p.name });
      }
      localStorage.setItem(cacheKey, localId);
      const original = storageService.saveProject.bind(storageService);
      storageService.saveProject = async (args) => {
        await original(args);
        if (!active || args.project.metadata.id !== localId) return;
        const document = cloudSnapshot(
          await (storageService as any).projectsAdapter.get(localId),
        );
        if (cloudSaveBlocked) {
          setStatus("Cloud conflict · reload required");
          return;
        }
        localStorage.setItem(pendingKey, String(revision));
        setStatus("Saving timeline…");
        saveQueue = saveQueue.then(async () => {
          if (cloudSaveBlocked) return;
          try {
            const result = await api(
              `/projects/${encodeURIComponent(project)}/editor`,
              { revision, document },
            );
            revision = result.revision;
            const latest = await (storageService as any).projectsAdapter.get(
              localId,
            );
            if (
              JSON.stringify(cloudSnapshot(latest)) === JSON.stringify(document)
            )
              localStorage.removeItem(pendingKey);
            else localStorage.setItem(pendingKey, String(revision));
            setStatus("Timeline saved · source files stay in this browser");
          } catch (e) {
            cloudSaveBlocked = true;
            setStatus(
              (e as Error).message +
                " Local edits are preserved; reload to reconnect.",
            );
          }
        });
      };
      if (active) {
        setStatus(
          cloudSaveBlocked
            ? "Cloud conflict · local edits preserved. Download a timeline backup or export."
            : "Timeline ready · source files stay in this browser",
        );
        setReady(true);
      }
    })().catch((e) => active && setError(e.message));
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    if (!ready) return;
    const listener = async (e: MessageEvent) => {
      if (
        e.origin !== location.origin ||
        e.source !== parentWindow ||
        typeof e.data?.type !== "string"
      )
        return;
      if (e.data.type === "videoflow:editor:resume") {
        if (e.data.requestId === flushRequest) {
          document.getElementById("root")!.inert = false;
          flushRequest = "";
        }
        return;
      }
      if (e.data.type === "videoflow:editor:flush") {
        const requestId = e.data.requestId;
        if (
          typeof requestId !== "string" ||
          !/^[-a-zA-Z0-9]{1,80}$/.test(requestId) ||
          flushRequest
        )
          return;
        flushRequest = requestId;
        document.getElementById("root")!.inert = true;
        try {
          const editor = EditorCore.getInstance();
          const deadline = Date.now() + 8000;
          while (editor.save.getIsDirty()) {
            await editor.save.flush();
            if (Date.now() > deadline)
              throw Error("Local save is still busy. Try leaving again.");
            if (editor.save.getIsDirty())
              await new Promise((resolve) => setTimeout(resolve, 25));
          }
          await editor.save.flush();
          await saveQueue;
          if (cloudSaveBlocked)
            throw Error(
              "Cloud save is paused. Your local edits remain here; download a backup before leaving.",
            );
          parent("videoflow:editor:flushed", { requestId, ok: true });
        } catch (e) {
          document.getElementById("root")!.inert = false;
          flushRequest = "";
          parent("videoflow:editor:flushed", {
            requestId,
            ok: false,
            error: (e as Error).message,
          });
        }
        return;
      }
      if (e.data.type !== "videoflow:editor:import") return;
      try {
        const item = e.data.asset;
        if (!item || typeof item.id !== "string") return;
        const r = await fetch(
          `/api/v1/media-assets/${encodeURIComponent(item.id)}/preview`,
          { credentials: "same-origin" },
        );
        if (!r.ok) throw Error("This source is no longer available.");
        const signed = await r.json();
        if (typeof signed.url !== "string")
          throw Error("No preview URL was returned.");
        const media = await fetch(signed.url);
        if (!media.ok) throw Error("Unable to read the source media.");
        const file = new File([await media.blob()], item.name || "source.mp4", {
          type: item.type || "video/mp4",
        });
        const assets = await processMediaAssets({ files: [file] });
        for (const asset of assets)
          await EditorCore.getInstance().media.addMediaAsset({
            projectId: localId,
            asset,
          });
        parent("videoflow:editor:imported", { id: item.id });
      } catch (e) {
        setStatus((e as Error).message);
      }
    };
    const parentWindow = window.parent;
    window.addEventListener("message", listener);
    parent("videoflow:editor:ready");
    return () => window.removeEventListener("message", listener);
  }, [ready]);
  if (error)
    return (
      <div className="vf-error" role="alert">
        <h1>Editor unavailable</h1>
        <p>{error}</p>
        <button onClick={() => location.reload()}>Try again</button>
      </div>
    );
  if (!ready)
    return (
      <div className="vf-error" role="status">
        Loading your editor…
      </div>
    );
  return (
    <EditorProvider projectId={localId}>
      <div className="vf-app">
        <header className="vf-header">
          <button
            aria-label="Back to projects"
            onClick={() => parent("videoflow:editor:exit")}
          >
            <ArrowLeft size={18} />
          </button>
          <strong>{name}</strong>
          <span role="status">{status}</span>
          <div className="vf-header-actions">
            <button onClick={() => void backup()}>Timeline backup</button>
            <button onClick={() => parent("videoflow:editor:storyboard")}>
              Storyboard
            </button>
            <button onClick={() => parent("videoflow:editor:generate")}>
              Generate clip
            </button>
            <button onClick={() => parent("videoflow:editor:sources")}>
              Project sources
            </button>
            <ExportButton />
          </div>
        </header>
        <SourceSafety />
        <div className="vf-workspace">
          <aside className="vf-tools">
            {[
              ["Media", FolderOpen],
              ["Text", Type],
              ["Captions", CaptionIcon],
              ["Effects", Sparkles],
              ["Settings", Settings],
            ].map(([label, Icon]: any) => (
              <button
                key={label}
                className={tab === label ? "active" : ""}
                onClick={() => setTab(label)}
              >
                <Icon size={20} />
                {label}
              </button>
            ))}
          </aside>
          <section className="vf-assets">
            {tab === "Media" ? (
              <MediaView />
            ) : tab === "Text" ? (
              <TextView />
            ) : tab === "Captions" ? (
              <Captions />
            ) : tab === "Effects" ? (
              <EffectsView />
            ) : (
              <SettingsView />
            )}
          </section>
          <section className="vf-preview">
            <PreviewPanel
              overlayControls={[]}
              overlayInstances={[]}
              onOverlayVisibilityChange={() => {}}
            />
          </section>
          <aside className="vf-properties">
            <PropertiesPanel />
          </aside>
        </div>
        <section className="vf-timeline">
          <Timeline />
        </section>
        <footer className="vf-foot">
          Powered by OpenCut Classic · Editing and exports run in your browser.{" "}
          <a href="/editor-app/notices.txt" target="_blank">
            Open-source notices
          </a>
          <button onClick={() => parent("videoflow:editor:publish")}>
            Publish an exported video
          </button>
        </footer>
      </div>
      <Toaster theme="dark" />
    </EditorProvider>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
