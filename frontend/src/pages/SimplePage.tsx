type Props = {
  title: string;
  text: string;
};

export function SimplePage({ title, text }: Props) {
  return (
    <div>
      <h2 className="text-3xl font-semibold text-ink">{title}</h2>
      <p className="mt-3 max-w-2xl text-sm text-slate-600">{text}</p>
    </div>
  );
}
