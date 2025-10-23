import { useRef, useState } from "react";
import { IRefPhaserGame, PhaserGame } from "./PhaserGame";
import { MainMenu } from "./game/scenes/MainMenu";
import { AuthProvider } from "./state/auth";
import { LoginButtons } from "./components/LoginButton";
import { EmailAuth } from "./components/EmailAuth";
import { Lobby } from "./components/Lobby";

function App() {
  const [canMoveSprite, setCanMoveSprite] = useState(true);
  const phaserRef = useRef<IRefPhaserGame | null>(null);
  const [spritePosition, setSpritePosition] = useState({ x: 0, y: 0 });

  const changeScene = () => {
    if (!phaserRef.current) return;
    const scene = phaserRef.current.scene as MainMenu;
    scene?.changeScene();
  };

  const moveSprite = () => {
    if (!phaserRef.current) return;
    const scene = phaserRef.current.scene as MainMenu;
    if (scene && scene.scene.key === "MainMenu") {
      scene.moveLogo(({ x, y }) => setSpritePosition({ x, y }));
    }
  };

  const addSprite = () => {
    if (!phaserRef.current) return;
    const scene = phaserRef.current.scene;
    if (scene) {
      const x = Phaser.Math.Between(64, scene.scale.width - 64);
      const y = Phaser.Math.Between(64, scene.scale.height - 64);
      const star = scene.add.sprite(x, y, "star");
      scene.add.tween({ targets: star, duration: 500 + Math.random() * 1000, alpha: 0, yoyo: true, repeat: -1 });
    }
  };

  const currentScene = (scene: Phaser.Scene) => setCanMoveSprite(scene.scene.key !== "MainMenu");

  return (
    <AuthProvider>
      <div id="app" style={{ display: "grid", gridTemplateColumns: "1fr 340px", height: "100vh" }}>
        <div>
          <PhaserGame ref={phaserRef} currentActiveScene={currentScene} />
        </div>
        <aside style={{ padding: 12, overflow: "auto", borderLeft: "1px solid #333" }}>
          <h3>Risk Clone</h3>
          <EmailAuth />
          <LoginButtons />
          <hr />
          <Lobby />
          <hr />
          <div>
            <button className="button" onClick={changeScene}>Change Scene</button>
          </div>
          <div>
            <button disabled={canMoveSprite} className="button" onClick={moveSprite}>Toggle Movement</button>
          </div>
          <div className="spritePosition">Sprite Position:
            <pre>{`{\n  x: ${spritePosition.x}\n  y: ${spritePosition.y}\n}`}</pre>
          </div>
          <div>
            <button className="button" onClick={addSprite}>Add New Sprite</button>
          </div>
        </aside>
      </div>
    </AuthProvider>
  );
}

export default App;
