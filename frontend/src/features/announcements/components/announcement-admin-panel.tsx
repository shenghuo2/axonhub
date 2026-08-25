import { useState } from 'react';
import { format } from 'date-fns';
import { Bell, BellPlus, LoaderCircle, Trash2, Users } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { PermissionGuard } from '@/components/permission-guard';
import { type Announcement, useAnnouncements, useDeleteAnnouncement } from '../data/announcements';
import { AnnouncementCreateDialog } from './announcement-create-dialog';

export function AnnouncementAdminPanel() {
  const { t } = useTranslation();
  const announcements = useAnnouncements();
  const deleteAnnouncement = useDeleteAnnouncement();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Announcement | null>(null);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    await deleteAnnouncement.mutateAsync(deleteTarget.id);
    setDeleteTarget(null);
  };

  return (
    <section className='border-border bg-card shrink-0 rounded-md border' aria-label={t('users.announcements.title')}>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3'>
        <div className='flex min-w-0 items-center gap-3'>
          <Bell className='text-primary size-5 shrink-0' />
          <div className='min-w-0'>
            <h3 className='text-sm font-semibold'>{t('users.announcements.title')}</h3>
            <p className='text-muted-foreground text-xs'>{t('users.announcements.description')}</p>
          </div>
        </div>
        <PermissionGuard requiredSystemScope='write_users'>
          <Button size='sm' onClick={() => setCreateOpen(true)}>
            <BellPlus />
            {t('users.announcements.publish')}
          </Button>
        </PermissionGuard>
      </div>

      <div className='max-h-64 divide-y overflow-y-auto'>
        {announcements.isLoading ? (
          <div className='text-muted-foreground flex items-center gap-2 px-4 py-5 text-sm'>
            <LoaderCircle className='size-4 animate-spin' />
            {t('common.loading')}
          </div>
        ) : announcements.data?.length ? (
          announcements.data.map((announcement) => (
            <div key={announcement.id} className='flex items-start gap-3 px-4 py-3'>
              <div className='min-w-0 flex-1'>
                <p className='whitespace-pre-wrap break-words text-sm'>{announcement.content}</p>
                <div className='text-muted-foreground mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
                  <span>{t('users.announcements.publishedAt', { time: format(new Date(announcement.createdAt), 'yyyy-MM-dd HH:mm') })}</span>
                  <span className='flex items-center gap-1'>
                    <Users className='size-3.5' />
                    {t('users.announcements.recipientCount', { count: announcement.recipientCount })}
                  </span>
                </div>
              </div>
              <PermissionGuard requiredSystemScope='write_users'>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant='ghost' size='icon' className='text-destructive hover:text-destructive' onClick={() => setDeleteTarget(announcement)}>
                      <Trash2 />
                      <span className='sr-only'>{t('users.announcements.delete')}</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('users.announcements.delete')}</TooltipContent>
                </Tooltip>
              </PermissionGuard>
            </div>
          ))
        ) : (
          <div className='text-muted-foreground px-4 py-5 text-sm'>{t('users.announcements.empty')}</div>
        )}
      </div>

      <AnnouncementCreateDialog open={createOpen} onOpenChange={setCreateOpen} />

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('users.announcements.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('users.announcements.deleteDescription')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteAnnouncement.isPending}>{t('common.buttons.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={deleteAnnouncement.isPending}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {deleteAnnouncement.isPending && <LoaderCircle className='animate-spin' />}
              {t('users.announcements.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
