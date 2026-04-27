export interface ComplianceControl {
  id: string;
  category: string;
  title: string;
  description: string;
  coveredLooksLike: string;
}

import controlsData from "../../sample-data/gdpr-requirements.json";

export const complianceControls: ComplianceControl[] = controlsData as ComplianceControl[];

export const controlCategories = [
  ...new Set(complianceControls.map((c) => c.category)),
];

export function getControlsByCategory(
  category: string
): ComplianceControl[] {
  return complianceControls.filter((c) => c.category === category);
}

export function getControlById(id: string): ComplianceControl | undefined {
  return complianceControls.find((c) => c.id === id);
}
