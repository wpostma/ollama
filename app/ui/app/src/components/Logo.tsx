export default function Logo() {
  return (
    <div className="flex mb-8 justify-center select-none">
      <img
        src="/delphi-ollama.png"
        alt="Delphi Ollama"
        width="78"
        height="78"
        className="select-none rounded-full"
        draggable={false}
      />
    </div>
  );
}
