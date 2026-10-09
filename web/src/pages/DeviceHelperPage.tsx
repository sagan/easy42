import React from "react";
import { useSearchParams } from "react-router-dom";
import { useMesh } from "../context/MeshContext";
import { DeviceHelperModal } from "../components/Modals/DeviceHelperModal";

export const DeviceHelperPage: React.FC = () => {
  const [searchParams] = useSearchParams();
  const { nodes } = useMesh();

  const initialNode = searchParams.get("node") || undefined;

  return (
    <DeviceHelperModal
      nodes={nodes}
      initialNode={initialNode}
    />
  );
};
