import { parseModelBadge } from "@/utils/modelBadge";

export function ModelBadge({
  modelName,
  size = "md",
}: {
  modelName: string;
  size?: "sm" | "md";
}) {
  const { tag, bgColor } = parseModelBadge(modelName);

  const sizeClasses =
    size === "sm"
      ? "h-5 w-5 text-[9px]"
      : "h-6 w-6 text-[10px]";

  return (
    <span
      className={`${sizeClasses} ${bgColor} text-white inline-flex items-center justify-center rounded-full font-bold leading-none shrink-0 tracking-tight`}
      title={modelName}
    >
      {tag}
    </span>
  );
}
