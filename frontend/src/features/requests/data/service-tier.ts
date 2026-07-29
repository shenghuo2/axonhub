export function isFastServiceTier(serviceTier: string | null | undefined): boolean {
  const normalized = serviceTier?.trim().toLowerCase();
  return normalized === 'fast' || normalized === 'priority';
}

export function formatServiceTier(serviceTier: string): string {
  const normalized = serviceTier.trim().toLowerCase();
  if (normalized === 'fast') return 'Fast';
  if (normalized === 'priority') return 'Fast (priority)';
  return serviceTier;
}
