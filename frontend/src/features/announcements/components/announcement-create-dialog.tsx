import { useEffect, useMemo, useState } from 'react';
import { CheckSquare, LoaderCircle, Square } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useAnnouncementRecipientAPIKeys, useCreateAnnouncement } from '../data/announcements';

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function AnnouncementCreateDialog({ open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [selectedAPIKeyIDs, setSelectedAPIKeyIDs] = useState<string[]>([]);
  const recipientAPIKeys = useAnnouncementRecipientAPIKeys(open);
  const createAnnouncement = useCreateAnnouncement();

  const recipientKeys = recipientAPIKeys.data ?? [];
  const allSelected = recipientKeys.length > 0 && recipientKeys.every((apiKey) => selectedAPIKeyIDs.includes(apiKey.id));
  const selectedCount = selectedAPIKeyIDs.length;

  useEffect(() => {
    if (!open) {
      setContent('');
      setSelectedAPIKeyIDs([]);
    }
  }, [open]);

  const selectedKeyIDs = useMemo(() => new Set(selectedAPIKeyIDs), [selectedAPIKeyIDs]);

  const toggleAPIKey = (id: string) => {
    setSelectedAPIKeyIDs((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  };

  const toggleAllAPIKeys = () => {
    setSelectedAPIKeyIDs(allSelected ? [] : recipientKeys.map((apiKey) => apiKey.id));
  };

  const handlePublish = async () => {
    await createAnnouncement.mutateAsync({
      content: content.trim(),
      apiKeyIDs: selectedAPIKeyIDs,
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>{t('users.announcements.publishTitle')}</DialogTitle>
          <DialogDescription>{t('users.announcements.publishDescription')}</DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='space-y-2'>
            <Label htmlFor='announcement-content'>{t('users.announcements.content')}</Label>
            <Textarea
              id='announcement-content'
              value={content}
              maxLength={2000}
              onChange={(event) => setContent(event.target.value)}
              placeholder={t('users.announcements.contentPlaceholder')}
              className='min-h-28 resize-y'
            />
            <div className='text-muted-foreground text-right text-xs'>{content.length}/2000</div>
          </div>

          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-3'>
              <Label>{t('users.announcements.recipients')}</Label>
              <Button type='button' variant='ghost' size='sm' onClick={toggleAllAPIKeys} disabled={recipientKeys.length === 0}>
                {allSelected ? <CheckSquare /> : <Square />}
                {allSelected ? t('users.announcements.clearSelection') : t('users.announcements.selectAll')}
              </Button>
            </div>
            <div className='border-input max-h-56 overflow-y-auto rounded-md border p-1'>
              {recipientAPIKeys.isLoading ? (
                <div className='text-muted-foreground flex min-h-24 items-center justify-center gap-2 text-sm'>
                  <LoaderCircle className='size-4 animate-spin' />
                  {t('common.loading')}
                </div>
              ) : recipientKeys.length === 0 ? (
                <div className='text-muted-foreground min-h-24 px-3 py-8 text-center text-sm'>{t('users.announcements.noRecipientKeys')}</div>
              ) : (
                recipientKeys.map((apiKey) => (
                  <label
                    key={apiKey.id}
                    className='hover:bg-accent flex cursor-pointer items-center gap-3 rounded-sm px-3 py-2 text-sm'
                  >
                    <Checkbox checked={selectedKeyIDs.has(apiKey.id)} onCheckedChange={() => toggleAPIKey(apiKey.id)} />
                    <span className='min-w-0 truncate font-mono'>{apiKey.name}</span>
                    <span className='text-muted-foreground shrink-0 font-mono text-xs'>{apiKey.id}</span>
                  </label>
                ))
              )}
            </div>
            <p className='text-muted-foreground text-xs'>{t('users.announcements.selectedCount', { count: selectedCount })}</p>
          </div>
        </div>

        <DialogFooter>
          <Button type='button' variant='outline' onClick={() => onOpenChange(false)} disabled={createAnnouncement.isPending}>
            {t('common.buttons.cancel')}
          </Button>
          <Button
            type='button'
            onClick={handlePublish}
            disabled={createAnnouncement.isPending || content.trim().length === 0 || selectedCount === 0}
          >
            {createAnnouncement.isPending && <LoaderCircle className='animate-spin' />}
            {t('users.announcements.publish')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
