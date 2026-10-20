import { useNavigate } from "@tanstack/react-router";
import { useCreativeProject } from "@/api/creative";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Select } from "@/components/ui/select";
import { BrandKitForm } from "@/creative/brand-kit-form";
import { RendersTab } from "@/creative/renders-tab";
import { useAllCreative } from "./shared";

export type LibrarySection = "pictures" | "brand";

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
  const projectId = given ?? all.projects[0]?.settings.projectId ?? 0;
  const project = useCreativeProject(projectId);
  const go = (next: { project?: number; section?: LibrarySection }) =>
    navigate({
      to: "/creative/library",
      search: {
        project: next.project ?? projectId,
        ...((next.section ?? section) === "brand" ? { section: "brand" as const } : {}),
      },
      replace: true,
    });

  return (
    <>
      <PageHeader
        title="Library"
        subtitle="Each project's pictures and brand. Ads and pages in every campaign are built from these."
        actions={
          all.projects.length > 0 && (
            <Select
              aria-label="Project"
              value={String(projectId)}
              onValueChange={(v) => go({ project: Number(v) })}
              options={all.projects.map((p) => ({
                value: String(p.settings.projectId),
                label: p.settings.projectName,
              }))}
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
        ]}
        className="mb-8"
      />
      {(all.isPending || (projectId > 0 && project.isPending)) && <LoadingRows rows={6} />}
      {all.error && <ErrorNote error={all.error} />}
      {!all.isPending && all.projects.length === 0 && (
        <Panel>
          <EmptyState title="No projects set up for creative yet" />
        </Panel>
      )}
      {project.data &&
        (section === "brand" ? (
          <BrandKitForm settings={project.data.settings} canEdit={canEdit} />
        ) : (
          <RendersTab project={project.data} canEdit={canEdit} />
        ))}
    </>
  );
}
