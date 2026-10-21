import { useNavigate } from "@tanstack/react-router";
import { useCreativeProject } from "@/api/creative";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Select } from "@/components/ui/select";
import { BrandKitForm } from "@/creative/brand-kit-form";
import { RendersTab } from "@/creative/renders-tab";
import { SettingsTab } from "@/creative/settings-tab";
import { SetUpStudio } from "../project";
import { useAllCreative } from "./shared";

export type LibrarySection = "pictures" | "brand" | "settings";

// Library keeps what every campaign of a project draws on: its pictures
// (renders, uploads and what Adx made) and its brand (logos, fonts, colours).
// Smartly's Media Library; campaigns pick from it rather than owning it.
export function CreativeLibraryPage({
  projectId: given,
  section = "pictures",
}: {
  projectId?: number;
  section?: LibrarySection;
}) {
  const navigate = useNavigate();
  const all = useAllCreative();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const choices = [
    ...all.projects.map((p) => ({ id: p.settings.projectId, name: p.settings.projectName })),
    ...(all.overview?.notSetUp ?? []).map((p) => ({ id: p.projectId, name: p.name })),
  ].sort((a, b) => a.name.localeCompare(b.name));
  const projectId = given ?? choices[0]?.id ?? 0;
  const setUp = all.projects.some((p) => p.settings.projectId === projectId);
  const project = useCreativeProject(setUp ? projectId : 0);
  const go = (next: { project?: number; section?: LibrarySection }) =>
    navigate({
      to: "/creative/library",
      search: {
        project: next.project ?? projectId,
        ...((next.section ?? section) !== "pictures" ? { section: next.section ?? section } : {}),
      },
      replace: true,
    });

  return (
    <>
      <PageHeader
        title="Library"
        subtitle="Each project's pictures and brand. Ads and pages in every campaign are built from these."
        actions={
          choices.length > 0 && (
            <Select
              aria-label="Project"
              value={String(projectId)}
              onValueChange={(v) => go({ project: Number(v) })}
              options={choices.map((p) => ({ value: String(p.id), label: p.name }))}
            />
          )
        }
      />
      <Segmented
        label="Library sections"
        value={section}
        onChange={(s) => go({ section: s })}
        options={[
          {
            value: "pictures",
            label: "Pictures",
            count: project.data?.renders.filter((r) => !r.hidden).length,
          },
          { value: "brand", label: "Brand" },
          { value: "settings", label: "Settings" },
        ]}
        className="mb-8"
      />
      {(all.isPending || (setUp && project.isPending)) && <LoadingRows rows={6} />}
      {all.error && <ErrorNote error={all.error} />}
      {!all.isPending && choices.length === 0 && (
        <Panel>
          <EmptyState title="No projects yet" />
        </Panel>
      )}
      {!all.isPending && projectId > 0 && !setUp && (
        <SetUpStudio projectId={projectId} canEdit={canEdit} />
      )}
      {setUp &&
        project.data &&
        (section === "brand" ? (
          <BrandKitForm settings={project.data.settings} canEdit={canEdit} />
        ) : section === "settings" ? (
          <SettingsTab settings={project.data.settings} canEdit={canEdit} />
        ) : (
          <RendersTab project={project.data} canEdit={canEdit} />
        ))}
    </>
  );
}
