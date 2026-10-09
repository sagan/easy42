import React from "react";
import { useSearchParams } from "react-router-dom";
import { useMesh } from "../context/MeshContext";
import { LookingGlassModal } from "../components/Modals/LookingGlassModal";

export const LookingGlassPage: React.FC = () => {
  const [searchParams] = useSearchParams();
  const { nodes } = useMesh();

  const initialNode = searchParams.get("node") || undefined;
  const initialTask = searchParams.get("task") || undefined;

  return (
    <LookingGlassModal
      nodes={nodes}
      initialNode={initialNode}
      initialTask={initialTask}
    />
  );
};
