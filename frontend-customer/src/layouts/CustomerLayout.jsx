import { Outlet } from "react-router";

function CustomerLayout() {
  return (
    <div>
      <header>
        <h1>CONNECT</h1>
      </header>

      <main>
        <Outlet />
      </main>
    </div>
  );
}

export default CustomerLayout;
