import { format } from 'date-fns';
import { Bell } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useMyAnnouncements } from '../data/announcements';

export function MyAnnouncements() {
  const { t } = useTranslation();
  const announcements = useMyAnnouncements();

  if (!announcements.data?.length) {
    return null;
  }

  return (
    <section className='border-primary/30 bg-primary/5 shrink-0 rounded-md border' aria-label={t('users.announcements.myTitle')}>
      <div className='flex items-center gap-3 border-b border-primary/20 px-4 py-3'>
        <Bell className='text-primary size-5 shrink-0' />
        <h3 className='text-sm font-semibold'>{t('users.announcements.myTitle')}</h3>
      </div>
      <div className='divide-y divide-primary/15'>
        {announcements.data.map((announcement) => (
          <div key={announcement.id} className='px-4 py-3'>
            <p className='whitespace-pre-wrap break-words text-sm'>{announcement.content}</p>
            <p className='text-muted-foreground mt-2 text-xs'>
              {t('users.announcements.publishedAt', { time: format(new Date(announcement.createdAt), 'yyyy-MM-dd HH:mm') })}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}
