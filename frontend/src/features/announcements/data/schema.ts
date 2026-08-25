import { z } from 'zod';

export const announcementSchema = z.object({
  id: z.string(),
  content: z.string(),
  createdAt: z.string(),
  recipientCount: z.number().int().nonnegative(),
});

export const announcementRecipientAPIKeySchema = z.object({
  id: z.string(),
  name: z.string(),
});

export const announcementListSchema = z.array(announcementSchema);
export const announcementRecipientAPIKeyListSchema = z.array(announcementRecipientAPIKeySchema);

export type Announcement = z.infer<typeof announcementSchema>;
export type AnnouncementRecipientAPIKey = z.infer<typeof announcementRecipientAPIKeySchema>;
