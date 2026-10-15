<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Textarea } from "$lib/components/ui/textarea";
  import chineseStarters from "./prompts/starters-zh.json";
  import { Label } from "$lib/components/ui/label";
  import * as Card from "$lib/components/ui/card";
  import {
    Clapperboard,
    BookOpen,
    Package,
    ArrowRight,
    LoaderCircle,
    Check,
  } from "@lucide/svelte";
  import { request } from "./api";
  let { userId }: { userId: string } = $props();
  const templates = [
    {
      id: "explainer",
      name: "Explainer",
      icon: BookOpen,
      description:
        "Introduce a problem, show the idea, and finish with a clear takeaway.",
      script:
        "Opening: Explain the problem your audience recognizes.\n\nMiddle: Introduce your idea and show how it helps.\n\nClosing: Summarize one clear takeaway.",
      scenes: [
        {
          title: "The problem",
          prompt:
            "Show the situation or challenge your audience will recognize. Establish the setting and the subject.",
          shot: "Opening context",
        },
        {
          title: "The idea",
          prompt:
            "Introduce the main idea. Show its effect through one clear action or visual example.",
          shot: "Demonstrate the idea",
        },
        {
          title: "The takeaway",
          prompt:
            "Resolve the opening challenge and finish on the key lesson or next step.",
          shot: "Closing takeaway",
        },
      ],
    },
    {
      id: "product-story",
      name: "Product story",
      icon: Package,
      description:
        "Start with a need, demonstrate a product, and show the result.",
      script:
        "Opening: Introduce the customer and what they need.\n\nDemonstration: Show the product being used in one specific situation.\n\nResult: Show the useful outcome and an honest next step.",
      scenes: [
        {
          title: "A real need",
          prompt:
            "Introduce a person and a specific need. Keep the setting and the task clear.",
          shot: "Customer context",
        },
        {
          title: "In use",
          prompt:
            "Show the product in use with a clear, believable action. Describe the product and its defining details.",
          shot: "Product demonstration",
        },
        {
          title: "The result",
          prompt:
            "Show the outcome of the action. End with a clean final view of the product or result.",
          shot: "Result and final frame",
        },
      ],
    },
    {
      id: "short-story",
      name: "Short story",
      icon: Clapperboard,
      description:
        "A four-scene narrative with a character, an obstacle, a turning point, and a resolution.",
      script:
        "Scene 1: Introduce the main character, their world, and what they want.\n\nScene 2: An obstacle makes the goal harder to reach.\n\nScene 3: A choice or discovery changes the direction of the story.\n\nScene 4: Resolve the central action and leave a final image.",
      scenes: [
        {
          title: "A world and a wish",
          prompt:
            "Establish the main character, their setting, and a clear goal.",
          shot: "Introduce the character",
        },
        {
          title: "An obstacle",
          prompt:
            "Show an obstacle interrupting the character’s attempt to reach their goal.",
          shot: "Present the conflict",
        },
        {
          title: "A turning point",
          prompt:
            "The character makes a choice or discovers something that changes what happens next.",
          shot: "The defining choice",
        },
        {
          title: "A resolution",
          prompt:
            "Resolve the central action and finish with a memorable final image.",
          shot: "Resolve the story",
        },
      ],
    },
  ];
  let templateGrid = $state<HTMLDivElement>();
  function revealSelected() {
    if (!templateGrid) return;
    const card = templateGrid.querySelector<HTMLElement>(".template-selected");
    if (!card) return;
    templateGrid.scrollLeft +=
      card.getBoundingClientRect().left -
      templateGrid.getBoundingClientRect().left -
      (templateGrid.clientWidth - card.getBoundingClientRect().width) / 2;
  }
  onMount(() => {
    if (!templateGrid) return;
    const observer = new ResizeObserver(revealSelected);
    observer.observe(templateGrid);
    revealSelected();
    return () => observer.disconnect();
  });
  let selected = $state(
    ["explainer", "product-story", "short-story"].includes(
      page.url.searchParams.get("starter") || "",
    )
      ? page.url.searchParams.get("starter")!
      : "explainer",
  );
  let projectName = $state("");
  let busy = $state(false);
  let error = $state("");
  let language = $state<"en" | "zh">("en");
  const localizedTemplates = $derived(
    templates.map((t) =>
      language === "zh"
        ? { ...t, ...chineseStarters[t.id as keyof typeof chineseStarters] }
        : t,
    ),
  );
  const current = $derived(
    localizedTemplates.find((template) => template.id === selected) ||
      localizedTemplates[0],
  );
  let overrides = $state<
    Record<
      string,
      {
        script: string;
        scenes: { title: string; prompt: string; shot: string }[];
      }
    >
  >({});
  const draft = $derived(
    overrides[`${language}:${selected}`] || {
      script: current.script,
      scenes: current.scenes,
    },
  );
  function updateDraft(patch: Partial<typeof draft>) {
    overrides = {
      ...overrides,
      [`${language}:${selected}`]: { ...draft, ...patch },
    };
  }
  function editScene(index: number, key: "title" | "prompt", value: string) {
    updateDraft({
      scenes: draft.scenes.map((s, i) =>
        i === index ? { ...s, [key]: value } : s,
      ),
    });
  }

  function choose(id: string) {
    selected = id;
    projectName = "";
    error = "";
  }
  async function create() {
    busy = true;
    error = "";
    try {
      const entities: any[] = [
        {
          id: "template-script",
          kind: "scripts",
          title: `${current.name} script`,
          content: draft.script,
          description:
            "Editable starter script. Replace the outline with your own story.",
          position: 1,
          duration: 0,
          metadata: { template: current.id },
        },
      ];
      for (const [index, scene] of draft.scenes.entries()) {
        const id = `template-scene-${index + 1}`;
        entities.push({
          id,
          kind: "scenes",
          title: scene.title,
          description: "Editable template scene.",
          prompt: scene.prompt,
          position: index + 1,
          duration: 5,
          metadata: { template: current.id },
        });
        entities.push({
          id: `template-shot-${index + 1}`,
          scene_id: id,
          kind: "shots",
          title: scene.shot,
          prompt: scene.prompt,
          position: 0,
          duration: 5,
          metadata: { template: current.id },
        });
      }
      const result = await request("/projects/import", {
        version: 1,
        project: {
          name: projectName.trim() || `${current.name} project`,
          description: `Created from the ${current.name.toLowerCase()} starter template.`,
        },
        entities,
      });
      if (!result.id)
        throw new Error(
          "The project was created but its identifier was not returned. Check your Projects page before trying again.",
        );
      localStorage.setItem(`videoflow.cloud.project.${userId}`, result.id);
      await goto(`/studio?project=${encodeURIComponent(result.id)}`);
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="page-heading">
  <h1>{language === "zh" ? "项目模板" : "Templates"}</h1>
  <p>
    {language === "zh"
      ? "从可编辑的结构开始，在画布中完善你的故事。"
      : "Start with an editable structure, then make it your own on the original VideoFlow canvas."}
  </p>
</div>
<div class="starter-language">
  <label for="starter-language">Template language / 模板语言</label><select
    id="starter-language"
    bind:value={language}
    onchange={() => (projectName = "")}
    ><option value="en">English</option><option value="zh"
      >简体中文 · Mandarin</option
    ></select
  >
</div>
{#if error}<div class="cloud-alert" role="alert">{error}</div>{/if}
<div class="template-grid" bind:this={templateGrid}>
  {#each localizedTemplates as template}<Card.Root
      class={`template-card ${selected === template.id ? "template-selected" : ""}`}
      ><div class="template-cover">
        <img
          src={`/artwork/${template.id}.webp`}
          alt=""
          width="960"
          height="540"
        /><span>AI concept artwork</span>
      </div>
      <Card.Header
        ><div class="template-card-top">
          <template.icon
            class="size-6 text-muted-foreground"
          />{#if selected === template.id}<Check class="size-4" />{/if}
        </div>
        <Card.Title>{template.name}</Card.Title><Card.Description
          >{template.description}</Card.Description
        ></Card.Header
      ><Card.Content
        ><p class="subtle">
          {template.scenes.length} scenes · {template.scenes.length} shots · editable
          script
        </p></Card.Content
      ><Card.Footer
        ><Button
          variant={selected === template.id ? "secondary" : "outline"}
          class="w-full"
          disabled={busy}
          aria-pressed={selected === template.id}
          onclick={() => choose(template.id)}
          >{selected === template.id ? "Selected" : "Choose template"}</Button
        ></Card.Footer
      ></Card.Root
    >{/each}
</div>
<details class="starter-customize">
  <summary
    >{language === "zh"
      ? "自定义剧本与场景"
      : "Customize script & scene directions"}</summary
  >
  <div>
    <label for="starter-script"
      >{language === "zh" ? "剧本大纲" : "Script outline"}</label
    ><Textarea
      id="starter-script"
      rows={7}
      maxlength={100000}
      value={draft.script}
      oninput={(e) => updateDraft({ script: e.currentTarget.value })}
    />{#each draft.scenes as scene, index}<section>
        <label for={`starter-scene-${index}`}
          >{language === "zh" ? "场景" : "Scene"} {index + 1}</label
        ><Input
          id={`starter-scene-${index}`}
          value={scene.title}
          maxlength={150}
          oninput={(e) => editScene(index, "title", e.currentTarget.value)}
        /><label for={`starter-direction-${index}`}
          >{language === "zh" ? "视觉方向" : "Visual direction"}</label
        ><Textarea
          id={`starter-direction-${index}`}
          rows={3}
          maxlength={8000}
          value={scene.prompt}
          oninput={(e) => editScene(index, "prompt", e.currentTarget.value)}
        />
      </section>{/each}<Button
      variant="ghost"
      onclick={() => {
        const next = { ...overrides };
        delete next[`${language}:${selected}`];
        overrides = next;
      }}>{language === "zh" ? "恢复默认内容" : "Reset this starter"}</Button
    >
  </div>
</details>
<div class="template-review">
  <section class="cloud-panel">
    <h2>{current.name} structure</h2>
    <ol>
      {#each draft.scenes as scene, index}<li>
          <span>{String(index + 1).padStart(2, "0")}</span>
          <div>
            <strong>{scene.title}</strong>
            <p>{scene.prompt}</p>
          </div>
        </li>{/each}
    </ol>
  </section>
  <form
    class="cloud-panel"
    onsubmit={(event) => {
      event.preventDefault();
      void create();
    }}
  >
    <h2>Create your project</h2>
    <Label for="template-project-name">Project name</Label><Input
      id="template-project-name"
      bind:value={projectName}
      placeholder={`${current.name} project`}
      maxlength={120}
      disabled={busy}
    />
    <p class="field-help">
      Creates a private project with a script, connected scenes, and shots. The
      canvas opens after the server creates the project.
    </p>
    <p class="field-help">
      No video generation starts and no generation credits are spent when you
      use a template.
    </p>
    <Button type="submit" disabled={busy}
      >{#if busy}<LoaderCircle class="size-4 animate-spin" />{/if}{language ===
      "zh"
        ? "使用模板创建项目"
        : "Create project from template"}<ArrowRight class="size-4" /></Button
    >
  </form>
</div>

<style>
  .starter-language {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
    margin-bottom: 22px;
  }
  .starter-language label {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .starter-language select {
    width: auto;
    max-width: 200px;
    font-size: 12px;
  }
  .starter-customize {
    margin-top: 24px;
    border-top: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
    padding: 16px 0;
  }
  .starter-customize summary {
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
  }
  .starter-customize > div {
    max-width: 760px;
    display: grid;
    gap: 10px;
    margin-top: 18px;
  }
  .starter-customize label {
    font-size: 12px;
    font-weight: 500;
  }
  .starter-customize section {
    display: grid;
    gap: 8px;
    padding: 16px 0;
    border-top: 1px solid var(--border);
  }
  @media (max-width: 767px) {
    .starter-language {
      justify-content: space-between;
    }
    .starter-language select {
      min-height: 44px;
    }
  }

  .template-cover {
    position: relative;
    aspect-ratio: 16/9;
    overflow: hidden;
    border-radius: 12px 12px 0 0;
  }
  .template-cover img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .template-cover span {
    position: absolute;
    left: 10px;
    bottom: 10px;
    font-size: 10px;
    padding: 4px 7px;
    border-radius: 4px;
    background: #0b0c0be6;
    color: #ebe9e2;
  }
  .template-grid :global(.template-card) {
    padding-top: 0;
    overflow: hidden;
  }
  .template-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 20px;
  }
  .template-grid :global(.template-card) {
    box-shadow: none;
    border-color: var(--border);
    background: var(--card);
  }
  .template-grid :global(.template-selected) {
    border-color: var(--foreground);
  }
  .template-card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
  }
  .template-grid :global([data-slot="card-title"]) {
    font-size: 17px;
  }
  .template-grid :global([data-slot="card-description"]) {
    font-size: 13px;
    line-height: 1.8;
  }
  .template-review {
    display: grid;
    grid-template-columns: 1.4fr 1fr;
    gap: 24px;
    align-items: start;
    margin-top: 28px;
  }
  .template-review ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .template-review li {
    display: flex;
    gap: 15px;
    padding: 18px 0;
    border-bottom: 1px solid var(--border);
  }
  .template-review li:last-child {
    border: 0;
    padding-bottom: 0;
  }
  .template-review li > span {
    font-size: 12px;
    color: var(--muted-foreground);
    padding-top: 2px;
  }
  .template-review li strong {
    font-size: 13px;
    font-weight: 500;
  }
  .template-review li p {
    font-size: 13px;
    color: var(--muted-foreground);
    margin-top: 6px;
    line-height: 1.8;
  }
  .template-review form > :global(button) {
    margin-top: 24px;
  }
  @media (max-width: 1000px) {
    .template-grid {
      grid-template-columns: repeat(3, minmax(260px, 1fr));
      overflow-x: auto;
      scroll-snap-type: x proximity;
      padding-bottom: 12px;
    }
    .template-grid :global(.template-card) {
      scroll-snap-align: start;
    }
    .template-review {
      grid-template-columns: 1fr;
    }
  }
</style>
