import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { graphqlRequest } from '@/gql/graphql';
import {
  announcementListSchema,
  announcementRecipientAPIKeyListSchema,
  announcementSchema,
  type Announcement,
  type AnnouncementRecipientAPIKey,
} from './schema';

const ANNOUNCEMENTS_QUERY = `
  query Announcements {
    announcements {
      id
      content
      createdAt
      recipientCount
    }
  }
`;

const MY_ANNOUNCEMENTS_QUERY = `
  query MyAnnouncements {
    myAnnouncements {
      id
      content
      createdAt
      recipientCount
    }
  }
`;

const ANNOUNCEMENT_RECIPIENT_API_KEYS_QUERY = `
  query AnnouncementRecipientAPIKeys {
    announcementRecipientAPIKeys {
      id
      name
    }
  }
`;

const CREATE_ANNOUNCEMENT_MUTATION = `
  mutation CreateAnnouncement($input: CreateAnnouncementInput!) {
    createAnnouncement(input: $input) {
      id
      content
      createdAt
      recipientCount
    }
  }
`;

const DELETE_ANNOUNCEMENT_MUTATION = `
  mutation DeleteAnnouncement($id: ID!) {
    deleteAnnouncement(id: $id)
  }
`;

export function useAnnouncements() {
  return useQuery({
    queryKey: ['announcements'],
    queryFn: async () => {
      const data = await graphqlRequest<{ announcements: Announcement[] }>(ANNOUNCEMENTS_QUERY);
      return announcementListSchema.parse(data.announcements);
    },
  });
}

export function useMyAnnouncements() {
  return useQuery({
    queryKey: ['my-announcements'],
    queryFn: async () => {
      const data = await graphqlRequest<{ myAnnouncements: Announcement[] }>(MY_ANNOUNCEMENTS_QUERY);
      return announcementListSchema.parse(data.myAnnouncements);
    },
  });
}

export function useAnnouncementRecipientAPIKeys(enabled: boolean) {
  return useQuery({
    queryKey: ['announcement-recipient-api-keys'],
    queryFn: async () => {
      const data = await graphqlRequest<{ announcementRecipientAPIKeys: AnnouncementRecipientAPIKey[] }>(
        ANNOUNCEMENT_RECIPIENT_API_KEYS_QUERY
      );
      return announcementRecipientAPIKeyListSchema.parse(data.announcementRecipientAPIKeys);
    },
    enabled,
  });
}

export function useCreateAnnouncement() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (input: { content: string; apiKeyIDs: string[] }) => {
      const data = await graphqlRequest<{ createAnnouncement: Announcement }>(CREATE_ANNOUNCEMENT_MUTATION, { input });
      return announcementSchema.parse(data.createAnnouncement);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['announcements'] });
      queryClient.invalidateQueries({ queryKey: ['my-announcements'] });
      toast.success(t('users.announcements.messages.publishSuccess'));
    },
    onError: () => {
      toast.error(t('common.errors.internalServerError'));
    },
  });
}

export function useDeleteAnnouncement() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const data = await graphqlRequest<{ deleteAnnouncement: boolean }>(DELETE_ANNOUNCEMENT_MUTATION, { id });
      return data.deleteAnnouncement;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['announcements'] });
      queryClient.invalidateQueries({ queryKey: ['my-announcements'] });
      toast.success(t('users.announcements.messages.deleteSuccess'));
    },
    onError: () => {
      toast.error(t('common.errors.internalServerError'));
    },
  });
}
